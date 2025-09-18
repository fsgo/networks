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
	"sync/atomic"
	"time"

	"github.com/xanygo/anygo/xcmd/xflag"
	"github.com/xanygo/anygo/xnet"
)

type Client struct {
	// ServerAddr 服务端的地址，必填，如 192.168.1.10:8080
	ServerAddr string

	// LocalAddr 期望对外发布的本地服务的地址，必填，如 127.0.0.1:8090
	LocalAddr string

	// Worker
	Worker int

	// ConnectTimeout 网络连接超时时间，可选
	ConnectTimeout time.Duration

	// Token 加密密码，可选
	Token string

	stopped atomic.Bool

	clientConnID atomic.Int64
	serverConnID atomic.Int64
}

func (c *Client) BindFlags() {
	xflag.EnvStringVar(&c.ServerAddr, "remote", "TT_C_remove", "127.0.0.1:8090", "remote tunnel server addr")
	xflag.EnvStringVar(&c.LocalAddr, "local", "TT_C_local", "127.0.0.1:8128", "local server addr tunnel to")
	xflag.EnvIntVar(&c.Worker, "worker", "TT_C_worker", 3, "worker number")
	xflag.EnvStringVar(&c.Token, "token", "TT_C_token", defaultToken, "token")
}

func (c *Client) Start() error {
	log.Println("Starting...")
	log.Println("Remote Addr=", c.ServerAddr, ", Local Addr=", c.LocalAddr)
	tl := &Tunneler{
		Worker:   c.Worker,
		Token:    c.Token,
		RemoteRW: c.connectToServer,
		LocalRW:  c.connectToClient,
	}
	return tl.Start()
}

func (c *Client) getConnectTimeout() time.Duration {
	if c.ConnectTimeout > 0 {
		return c.ConnectTimeout
	}
	return 10 * time.Second
}

var helloMsgReq = []byte("Hello")
var helloMsgResp = []byte("OK")

func (c *Client) checkServerToken(rw io.ReadWriteCloser) error {
	// 单独发送一个消息给 server，用于检验 token
	// 若 server 解析不出来，server 会主动断开连接
	if _, err := rw.Write(helloMsgReq); err != nil {
		return fmt.Errorf("write helloMsgReq failed: %w", err)
	}
	bf := make([]byte, len(helloMsgResp))
	if _, err := io.ReadFull(rw, bf); err != nil {
		return fmt.Errorf("read helloMsgResp failed: %w", err)
	}
	if !bytes.Equal(bf, helloMsgResp) {
		return fmt.Errorf("invalid helloMsgResp: %q", bf)
	}
	return nil
}

func (c *Client) connectToServer() io.ReadWriteCloser {
	for i := 0; ; i++ {
		rw := c.connectTo("server", c.ServerAddr, c.serverConnID.Add(1))
		if rw == nil {
			return nil
		}
		rw = rwWithToken(rw, c.Token)
		if err := c.checkServerToken(rw); err != nil {
			_ = rw.Close()
			log.Println("[connect_server]", rwInfo(rw), "check server conn failed,", err)
			wait(i)
			continue
		}
		return rw
	}
}

func (c *Client) connectToClient() io.ReadWriteCloser {
	return c.connectTo("local", c.LocalAddr, c.clientConnID.Add(1))
}

func (c *Client) connectTo(tp string, address string, id int64) io.ReadWriteCloser {
	for i := 0; !c.stopped.Load(); i++ {
		msg := fmt.Sprintf("[connect_%s] [%d] [try=%d] %s", tp, id, i, address)
		start := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), c.getConnectTimeout())
		conn, err := xnet.DialContext(ctx, "tcp", address)
		cost := time.Since(start)
		cancel()
		if err != nil {
			log.Println(msg, ", failed, err=", err, ", cost=", cost.String())
			wait(i)
			continue
		}
		log.Println(msg, ", success, cost=", cost.String())
		return conn
	}
	return nil
}
