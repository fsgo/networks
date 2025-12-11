// Copyright(C) 2023 github.com/fsgo  All Rights Reserved.
// Author: hidu <duv123@gmail.com>
// Date: 2023/10/7

package tcptunnel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync/atomic"
	"time"

	"github.com/xanygo/anygo/cli/xflag"
	"github.com/xanygo/anygo/ds/xsync"
	"github.com/xanygo/anygo/xio"
	"github.com/xanygo/anygo/xnet/xrps"

	"github.com/fsgo/networks/internal"
)

func NewServer() *Server {
	return &Server{}
}

// Server 用于提供外网服务
type Server struct {
	// ListenOut 对外转发的监听地址，必填
	ListenOut string

	// ListenClient 为 Client 准备的监听地址，必填
	ListenClient string

	// Token 加密密码，可选
	Token string

	clientMux    xsync.Value[*xio.Mux]
	clientConnCh chan struct{}

	cntStreamTotal    atomic.Int64 // 累计创建的 stream 总数
	cntStreamErrTotal atomic.Int64 // stream 读写后 err!=nil 的总数

	cntOuterNow   atomic.Int64 // 连接中的 OutHandler
	cntOuterTotal atomic.Int64

	cntClientNow   atomic.Int64 // 连接中的 client
	cntClientTotal atomic.Int64 // client 累计连接数
}

func (s *Server) BindFlags() {
	xflag.EnvStringVar(&s.ListenOut, "out", "TT_S_out", "127.0.0.1:8100", "addr export")
	xflag.EnvStringVar(&s.ListenClient, "in", "TT_S_in", ":8090", "addr for tunnel client")
	xflag.EnvStringVar(&s.Token, "token", "TT_S_token", defaultToken, "token")
}

func (s *Server) Start() error {
	s.clientConnCh = make(chan struct{}, 1)

	eg := &xsync.WaitGo{}
	eg.GoErr(s.startListenOut)
	eg.GoErr(s.startListenClient)
	eg.GoErr(s.startTrace)
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

	fs := &xrps.AnyServer{
		Handler: xrps.HandleFunc(func(ctx context.Context, conn net.Conn) {
			id := connID.Add(1)
			s.outHandler(ctx, conn, id)
		}),
	}
	return fs.Serve(l)
}

func (s *Server) outHandler(ctx context.Context, localConn net.Conn, id int64) {
	s.cntOuterNow.Add(1)
	s.cntOuterTotal.Add(1)
	defer func() {
		s.cntOuterNow.Add(-1)
		localConn.Close()
	}()

	msg := fmt.Sprintf("[server conn] [%d] ", id) + rwInfo(localConn)

	log.Println(msg)
	start := time.Now()

	var stream *xio.MuxStream
	var err error

	for i := 0; i < 3; i++ {
		mx := s.clientMux.Load()
		if mx == nil {
			err = errors.New("clientMux is nil, pls check tunnel-client")
			log.Println("clientMux is nil, try=", i)
			s.summoning()
			select {
			case <-ctx.Done():
			case <-time.After(100 * time.Millisecond):
			}
			continue
		}
		stream, err = mx.Open()
		if err != nil {
			log.Println("clientMux open failed:", err)
			mx.Close()
			s.summoning()
			continue
		}
		s.cntStreamTotal.Add(1)
		break
	}

	if stream != nil {
		log.Println(msg, "start RWCopy, sid=", stream.ID())
		err = internal.RWCopy(stream, localConn)
		if err != nil {
			s.cntStreamErrTotal.Add(1)
		}
	}
	cost := time.Since(start)
	log.Println(msg, "closed, err=", err, ",cost=", cost.String(), ",cntOuter=", s.cntOuterNow.Load())
}

func (s *Server) summoning() {
	select {
	case <-s.clientConnCh:
	default:
	}
}

func (s *Server) startListenClient() error {
	log.Println("Listen tunnelInServer at:", s.ListenClient)
	l, err := net.Listen("tcp", s.ListenClient)
	if err != nil {
		return err
	}

	var connID atomic.Int64
	fs := &xrps.AnyServer{
		Handler: xrps.HandleFunc(func(ctx context.Context, conn net.Conn) {
			id := connID.Add(1)
			s.clientHandler(ctx, conn, id)
		}),
	}
	return fs.Serve(l)
}

// clientHandler 处理 tcp-tunnel-client 发起的连接
func (s *Server) clientHandler(ctx context.Context, conn net.Conn, id int64) {
	s.cntClientNow.Add(1)
	defer s.cntClientNow.Add(-1)
	s.cntClientTotal.Add(1)

	start := time.Now()
	msg := fmt.Sprintf("[tunnel client conn] [%d] ", id) + rwInfo(conn)
	log.Println(msg, "ClientConnecting=", s.cntClientNow.Load())

	rw := rwWithToken(conn, s.Token)

	// 校验是否由客户端发送请求
	if err1 := s.checkClientConn(conn, rw); err1 != nil {
		_ = rw.Close()
		log.Println(msg, "invalid client, err=", err1)
		return
	}

	tk := time.NewTicker(time.Second)
	defer tk.Stop()

	for idx := 0; ; idx++ {
		select {
		case s.clientConnCh <- struct{}{}:
			waitTime := time.Since(start)
			if err := isBadConn(conn); err != nil {
				conn.Close()
				log.Println(msg, "loop=", idx, ",isBadConn:", err, "wait=", waitTime.String())
				return
			}
			muc := xio.NewMux(false, rw)
			old := s.clientMux.Swap(muc)
			log.Println(msg, "replace as NewMux, loop=", idx, "wait=", waitTime.String())
			if old != nil {
				_ = old.Close()
			}
			return
		case <-tk.C:
			// 每秒检查一下当前连接是否完好，若连接已经断开则释放掉
			waitTime := time.Since(start)
			if err := isBadConn(conn); err != nil {
				conn.Close()
				log.Println(msg, "loop=", idx, ",isBadConn:", err, "wait=", waitTime.String())
				return
			}
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

func (s *Server) startTrace() error {
	tm := time.NewTicker(5 * time.Second)
	defer tm.Stop()

	for {
		<-tm.C
		info := map[string]any{
			"StreamCreated": s.cntStreamTotal.Load(),
			"StreamErrs":    s.cntStreamErrTotal.Load(),

			"OuterConnecting": s.cntOuterNow.Load() + 1,
			"OuterConnected":  s.cntOuterTotal.Load(),

			"ClientConnecting": s.cntClientNow.Load(),
			"ClientConnected":  s.cntClientTotal.Load(),
		}
		bf, _ := json.Marshal(info)
		log.Println("[server.trace]", string(bf))
	}
}
