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

var client = &tcptunnel.Client{}

func init() {
	log.SetPrefix(fmt.Sprintf("[tcp-tunnel-client][pid=%d] ", os.Getpid()))
	client.BindFlags()
}

func main() {
	flag.Parse()
	err := client.Start()
	log.Fatalln("tcp-tunnel-client exit:", err)
}
