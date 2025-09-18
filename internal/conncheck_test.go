// Copyright(C) 2021 github.com/fsgo  All Rights Reserved.
// Author: fsgo
// Date: 2021/6/4

//go:build linux || darwin || dragonfly || freebsd || netbsd || openbsd || solaris || illumos
// +build linux darwin dragonfly freebsd netbsd openbsd solaris illumos

package internal

import (
	"net"
	"net/http/httptest"
	"testing"
	"time"
)

func Test_connCheck(t *testing.T) {
	ts := httptest.NewServer(nil)
	defer ts.Close()

	t.Run("good conn", func(t *testing.T) {
		conn, err := net.DialTimeout(ts.Listener.Addr().Network(), ts.Listener.Addr().String(), time.Second)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer conn.Close()
		if err = ConnCheck(conn); err != nil {
			t.Fatal(err.Error())
		}
		conn.Close()

		if err = ConnCheck(conn); err == nil {
			t.Fatal("expect has error")
		}
	})

	t.Run("bad conn 2", func(t *testing.T) {
		conn, err := net.DialTimeout(ts.Listener.Addr().Network(), ts.Listener.Addr().String(), time.Second)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer conn.Close()

		ts.Close()

		if err = ConnCheck(conn); err == nil {
			t.Fatal("expect has err")
		}
	})
}
