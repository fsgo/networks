// Copyright(C) 2023 github.com/fsgo  All Rights Reserved.
// Author: hidu <duv123@gmail.com>
// Date: 2023/10/7

package tcptunnel

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"time"
)

func wait(n int) {
	if n < 10 {
		time.Sleep(200 * time.Millisecond)
		return
	}
	time.Sleep(time.Second)
}

func newStream(token string) cipher.Stream {
	m := md5.New()
	m.Write([]byte(token))
	key := []byte(hex.EncodeToString(m.Sum(nil)))
	block, err := aes.NewCipher(key)
	if err != nil {
		panic(err)
	}
	var iv [aes.BlockSize]byte
	return cipher.NewOFB(block, iv[:])
}

func rwWithToken(rw io.ReadWriteCloser, token string) io.ReadWriteCloser {
	if token == "" || token == "no" {
		return rw
	}
	writer := &cipher.StreamWriter{
		S: newStream(token),
		W: rw,
	}
	reader := &cipher.StreamReader{
		S: newStream(token),
		R: rw,
	}

	w := &rwWrapper{
		w:   writer,
		r:   reader,
		c:   rw,
		msg: rwInfo(rw),
	}
	return w
}

var _ io.ReadWriteCloser = (*rwWrapper)(nil)

type rwWrapper struct {
	w   io.WriteCloser
	r   io.Reader
	c   io.Closer
	msg string
}

func (e *rwWrapper) Read(p []byte) (n int, err error) {
	return e.r.Read(p)
}

func (e *rwWrapper) Write(p []byte) (n int, err error) {
	return e.w.Write(p)
}

func (e *rwWrapper) Close() error {
	_ = e.w.Close()
	return e.c.Close()
}

func (e *rwWrapper) String() string {
	return e.msg
}

func rwInfo(rd io.Reader) string {
	if conn, ok := rd.(net.Conn); ok {
		return fmt.Sprintf("local=%q, remote=%s", conn.LocalAddr().String(), conn.RemoteAddr().String())
	}
	if fs, ok := rd.(fmt.Stringer); ok {
		return fs.String()
	}
	return fmt.Sprintf("%#v", rd)
}
