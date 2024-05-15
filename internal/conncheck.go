// Copyright(C) 2021 github.com/fsgo  All Rights Reserved.
// Author: fsgo
// Date: 2021/6/4

// https://github.com/go-sql-driver/mysql/blob/master/conncheck.go

//go:build linux || darwin || dragonfly || freebsd || netbsd || openbsd || solaris || illumos
// +build linux darwin dragonfly freebsd netbsd openbsd solaris illumos

package internal

import (
	"errors"
	"io"
	"net"
	"syscall"
)

var errUnexpectedRead = errors.New("unexpected read from socket")
var errConnNil = errors.New("conn is nil")

func ConnCheck(conn net.Conn) error {
	if conn == nil {
		return errConnNil
	}
	var sysErr error

	sysConn, ok := conn.(syscall.Conn)
	if !ok {
		return nil
	}
	rawConn, err := sysConn.SyscallConn()
	if err != nil {
		return err
	}

	err = rawConn.Read(func(fd uintptr) bool {
		var buf [1]byte
		n, errRead := syscall.Read(int(fd), buf[:])
		switch {
		case n == 0 && errRead == nil:
			sysErr = io.EOF
		case n > 0:
			sysErr = errUnexpectedRead
		case errors.Is(errRead, syscall.EAGAIN) || errors.Is(errRead, syscall.EWOULDBLOCK):
			sysErr = nil
		default:
			sysErr = errRead
		}
		return true
	})
	if err != nil {
		return err
	}

	return sysErr
}
