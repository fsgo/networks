// Copyright(C) 2023 github.com/fsgo  All Rights Reserved.
// Author: hidu <duv123@gmail.com>
// Date: 2023/10/7

package tcptunnel

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"sync/atomic"
	"time"

	"github.com/fsgo/fsgo/fsflag"
	"github.com/fsgo/fsgo/fsserver"
	"github.com/fsgo/fsgo/fssync/fsatomic"
	"golang.org/x/sync/errgroup"

	"github.com/fsgo/networks/internal"
)

type Server struct {
	// ListenOut 对外转发的监听地址，必填
	ListenOut string

	// ListenClient 为 Client 准备的监听地址，必填
	ListenClient string

	// Token 加密密码，可选
	Token string

	ClientExpire time.Duration

	Size int

	clientConns chan io.ReadWriteCloser

	lastUse fsatomic.TimeStamp
}

func (s *Server) BindFlags() {
	ef := fsflag.EnvFlags{}
	ef.StringVar(&s.ListenOut, "out", "TT_S_out", "127.0.0.1:8100", "addr export")
	ef.StringVar(&s.ListenClient, "in", "TT_S_in", ":8090", "addr for tunnel client")
	ef.StringVar(&s.Token, "token", "TT_S_token", defaultToken, "token")
	ef.IntVar(&s.Size, "size", "TT_S_size", 10, "connection chan buffer size")
	ef.DurationVar(&s.ClientExpire, "exp", "TT_S_exp", 10*time.Minute, "client connections expire")
}

func (s *Server) getSize() int {
	if s.Size > 0 {
		return s.Size
	}
	return 10
}

func (s *Server) Start() error {
	s.clientConns = make(chan io.ReadWriteCloser, s.getSize())

	eg := &errgroup.Group{}
	eg.Go(s.startListenOut)
	eg.Go(s.startListenClient)
	return eg.Wait()
}

func (s *Server) startListenOut() error {
	// 对外暴露的端口，最终用户通过访问此端口来访问到内网的端口
	log.Println("Listen tunnelOutServer at:", s.ListenOut)
	l, err := net.Listen("tcp", s.ListenOut)
	if err != nil {
		return err
	}
	var connID atomic.Int64

	fs := &fsserver.AnyServer{
		Handler: fsserver.HandleFunc(func(ctx context.Context, conn net.Conn) {
			id := connID.Add(1)
			s.outHandler(ctx, conn, id)
		}),
	}
	go s.cleanOldClients()
	return fs.Serve(l)
}

func (s *Server) outHandler(ctx context.Context, conn net.Conn, id int64) {
	msg := fmt.Sprintf("[server conn] [%d] ", id) + rwInfo(conn) + fmt.Sprintf(" client.len=%d", len(s.clientConns))

	log.Println(msg)
	start := time.Now()

	var exitErr error
	defer func() {
		cost := time.Since(start)
		log.Println(msg, "closed, err=", exitErr, ",cost=", cost.String())
	}()

	// tunnel-client 的 net.conn
	var remote io.ReadWriteCloser
	for idx := 0; idx < 100; idx++ {
		select {
		case remote = <-s.clientConns:
		case <-ctx.Done():
			exitErr = context.Cause(ctx)
			return
		}

		exitErr = isBadConn(remote)

		if exitErr == nil {
			break
		}
		_ = remote.Close()
		log.Println(msg, "ignore bad conn, err=", exitErr, ",try=", idx)
	}

	s.lastUse.Store(time.Now())

	exitErr = internal.RWCopy(remote, conn)
}

func (s *Server) cleanOldClients() {
	if s.ClientExpire <= time.Second {
		return
	}
	tm := time.NewTimer(5 * time.Second)
	var dropID atomic.Int64

	checkDrop := func() {
		defer tm.Reset(time.Second)
		dur := time.Since(s.lastUse.Load())
		if dur < s.ClientExpire {
			return
		}

		defer func() {
			s.lastUse.Store(time.Now())
		}()

		for i := 0; i < len(s.clientConns); i++ {
			select {
			case in := <-s.clientConns:
				id := dropID.Add(1)
				e2 := in.Close()
				log.Println("drop idle connection, ",
					"drop_total=", id,
					"idle_duration", dur.String(),
					rwInfo(in),
					"close=", e2,
				)

			default:
				log.Println("no connections when check idle")
				return
			}
		}
	}

	for range tm.C {
		checkDrop()
	}
}

func (s *Server) startListenClient() error {
	log.Println("Listen tunnelInServer at:", s.ListenClient)
	l, err := net.Listen("tcp", s.ListenClient)
	if err != nil {
		return err
	}

	var connID atomic.Int64
	fs := &fsserver.AnyServer{
		Handler: fsserver.HandleFunc(func(ctx context.Context, conn net.Conn) {
			id := connID.Add(1)
			s.clientHandler(ctx, conn, id)
		}),
	}
	return fs.Serve(l)
}

func (s *Server) clientHandler(ctx context.Context, conn net.Conn, id int64) {
	msg := fmt.Sprintf("[client conn] [%d] ", id) + rwInfo(conn)

	rw := rwWithToken(conn, s.Token)

	// 校验是否由客户端发送请求
	if err1 := s.checkClientConn(conn, rw); err1 != nil {
		_ = rw.Close()
		log.Println(msg, "invalid client, err=", err1)
		return
	}

	ctx1, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	select {
	case s.clientConns <- rw:
		log.Println(msg, "received, client.len=", len(s.clientConns))
		return
	case <-ctx1.Done():
		// buffer 满的情况下，尝试将旧的连接取出，新的连接放进去
		select {
		case in := <-s.clientConns:
			_ = in.Close()
			select {
			case s.clientConns <- rw:
				log.Println(msg, "replaced, client.len=", len(s.clientConns))
			default:
				log.Println(msg, "dropped by no buffer")
				_ = rw.Close()
			}
		default:
			_ = rw.Close()
			log.Println(msg, "dropped by replaced")
		}
	}
}

func (s *Server) checkClientConn(conn net.Conn, rw io.ReadWriteCloser) error {
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	bf := make([]byte, len(helloMsgReq))
	if _, err1 := io.ReadFull(rw, bf); err1 != nil {
		return fmt.Errorf("read helloMsgReq failed: %w", err1)
	}
	if !bytes.Equal(bf, helloMsgReq) {
		return fmt.Errorf("invalid helloMsgReq: %q", bf)
	}
	if _, err2 := rw.Write(helloMsgResp); err2 != nil {
		return fmt.Errorf("write helloMsgResp failed: %w", err2)
	}
	_ = conn.SetDeadline(time.Time{})
	return nil
}
