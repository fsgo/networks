// Copyright(C) 2023 github.com/fsgo  All Rights Reserved.
// Author: hidu <duv123@gmail.com>
// Date: 2023/11/3

package main

import (
	"context"
	"flag"
	"log"
	"net"
	"syscall"
	"time"
)

var addr = flag.String("l", ":8000", "listen addr")

func main() {
	flag.Parse()
	lc := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			c.Control(func(fd uintptr) {
				time.Sleep(time.Second)
			})
			c.Write(func(fd uintptr) (done bool) {
				time.Sleep(time.Second)
				return true
			})
			return nil
		},
	}
	l, err := lc.Listen(context.Background(), "tcp", *addr)
	if err != nil {
		log.Fatalln(err)
	}
	defer l.Close()
	select {}
}
