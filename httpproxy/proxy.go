// Copyright(C) 2023 github.com/fsgo  All Rights Reserved.
// Author: hidu <duv123@gmail.com>
// Date: 2023/10/22

package httpproxy

import "net/http"

type Server struct {
	Listen   string
	Location []Location
}

var _ http.Handler = Location{}

type Location struct {
	Path string
	Pass string
}

func (l Location) ServeHTTP(w http.ResponseWriter, r *http.Request) {}
