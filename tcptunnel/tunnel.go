// Copyright(C) 2023 github.com/fsgo  All Rights Reserved.
// Author: hidu <duv123@gmail.com>
// Date: 2023/10/7

package tcptunnel

import (
	"fmt"
	"io"
	"log"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/fsgo/networks/internal"
)

const defaultToken = "hello:tcp-tunnel"

type Tunneler struct {
	RemoteRW func() io.ReadWriteCloser
	LocalRW  func() io.ReadWriteCloser

	Worker int

	remoteRWChan chan io.ReadWriteCloser
	stopped      atomic.Bool
}

func (c *Tunneler) getWorker() int {
	if c.Worker > 0 {
		return c.Worker
	}
	return 3
}

func (c *Tunneler) Start() error {
	c.remoteRWChan = make(chan io.ReadWriteCloser, c.getWorker())
	var eg errgroup.Group
	eg.Go(c.connectToRemote)
	eg.Go(c.connectToLocal)
	return eg.Wait()
}

func (c *Tunneler) connectToRemote() error {
	ec := make(chan error, c.getWorker())
	for i := 0; i < c.getWorker(); i++ {
		go func() {
			defer func() {
				ec <- fmt.Errorf("worker %d exit", i)
			}()
			for !c.stopped.Load() {
				rw := c.RemoteRW()
				if rw == nil {
					break
				}
				c.remoteRWChan <- rw
			}
		}()
	}
	return <-ec
}

func (c *Tunneler) connectToLocal() error {
	ec := make(chan error, c.getWorker())
	for i := 0; i < c.getWorker(); i++ {
		go func() {
			defer func() {
				ec <- fmt.Errorf("worker %d exit", i)
			}()
			for rw := range c.remoteRWChan {
				lc := c.LocalRW()
				if lc == nil {
					break
				}
				start := time.Now()
				err1 := internal.RWCopy(rw, lc)
				cost := time.Since(start)
				log.Println("copy remote to local, cost=", cost.String(), "err=", err1)
			}
		}()
	}
	return <-ec
}

func (c *Tunneler) Stop() {
	c.stopped.Store(true)
}
