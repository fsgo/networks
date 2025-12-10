// Copyright(C) 2023 github.com/fsgo  All Rights Reserved.
// Author: hidu <duv123@gmail.com>
// Date: 2023/10/8

package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/fsgo/networks/tcptunnel"
)

var server = tcptunnel.NewServer()

func init() {
	log.SetPrefix(fmt.Sprintf("[tcp-tunnel-server][pid=%d] ", os.Getpid()))
	server.BindFlags()
}

func main() {
	flag.Parse()
	err := server.Start()
	log.Fatalln("tcp-tunnel-server exit:", err)
}
