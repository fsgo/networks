// Copyright(C) 2023 github.com/fsgo  All Rights Reserved.
// Author: hidu <duv123@gmail.com>
// Date: 2023/10/7

package tcptunnel

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"sort"
	"sync/atomic"
	"time"

	"github.com/xanygo/anygo/ds/xsync"
	"github.com/xanygo/anygo/safely"
	"github.com/xanygo/anygo/xio"

	"github.com/fsgo/networks/internal"
)

const defaultToken = "hello:tcp-tunnel"

type Tunneler struct {
	// RemoteRW 和远端 Tunneler client 或者 server 的连接
	RemoteRW func() io.ReadWriteCloser

	// LocalRW 和本地其他 server（待穿透的实际服务），如 nginx 等的连接
	LocalRW func() io.ReadWriteCloser

	Worker int

	Token string

	stopped atomic.Bool

	cntStreamNow   atomic.Int64
	cntStreamTotal atomic.Int64

	cntRemoteTotal atomic.Int64 // 连接到远程 server 的总数
}

func (c *Tunneler) getWorker() int {
	if c.Worker > 0 {
		return c.Worker
	}
	return 1
}

func (c *Tunneler) Start() error {
	var eg xsync.WaitGo
	eg.GoErr(c.connectToLocal)
	eg.Go(c.startTrace)
	return eg.Wait()
}

func (c *Tunneler) connectToLocal() error {
	ec := make(chan error, c.getWorker())
	for i := 0; i < c.getWorker(); i++ {
		go c.localWorker(i, ec)
	}
	return <-ec
}

func (c *Tunneler) localWorker(id int, ec chan<- error) {
	defer func() {
		ec <- fmt.Errorf("worker %d exit", id)
	}()

	onRemote := func(conn io.ReadWriteCloser) {
		defer conn.Close()

		muc := xio.NewMux(true, conn)
		defer muc.Close()

		go func() {
			tm := time.NewTicker(5 * time.Second)
			for range tm.C {
				var ids []int
				muc.Range(func(s *xio.MuxStream) bool {
					ids = append(ids, int(s.ID()))
					return true
				})
				sort.Ints(ids)
				log.Println("Tunneler MuxStream.IDS=", ids)
			}
		}()

		var wg xsync.WaitGo
		for {
			stream, err := muc.Accept() // 接受到一个 server 传过来的连接
			if err != nil {
				log.Println("mux.Accept err:", err)
				break
			}
			c.cntStreamTotal.Add(1)
			num := c.cntStreamNow.Add(1)
			log.Printf("mux.Accept stream sid=%d, cntStream=%d", stream.ID(), num)
			wg.Go(func() {
				defer func() {
					stream.Close()
					c.cntStreamNow.Add(-1)
				}()

				// 创建到本地端口的连接
				localConn := c.LocalRW()
				if localConn == nil {
					return
				}
				start := time.Now()
				log.Printf("start copy remote (sid=%d) to local", stream.ID())
				err1 := internal.RWCopy(stream, localConn)
				cost := time.Since(start)
				log.Printf("copied remote (sid=%d) to local, cost=%s, err=%v", stream.ID(), cost.String(), err1)
			})
		}
		muc.Close()
		wg.Wait()
	}

	for !c.stopped.Load() {
		remoteConn := c.RemoteRW()
		if remoteConn == nil {
			continue
		}
		c.cntRemoteTotal.Add(1)
		if err := isBadConn(remoteConn); err != nil {
			_ = remoteConn.Close()
			log.Println("remote conn is bad, err=", err)
			continue
		}
		safely.RunVoid(func() {
			onRemote(remoteConn)
		})
	}
}

func (c *Tunneler) Stop() {
	c.stopped.Store(true)
}

func (c *Tunneler) startTrace() {
	tm := time.NewTicker(5 * time.Second)
	defer tm.Stop()

	for !c.stopped.Load() {
		<-tm.C
		info := map[string]any{
			"StreamWorking": c.cntStreamNow.Load(),
			"StreamTotal":   c.cntStreamTotal.Load(),

			"RemoteConnected": c.cntRemoteTotal.Load(),
		}
		bf, _ := json.Marshal(info)
		log.Println("[Tunneler.trace]", string(bf))
	}
}
