// Copyright(C) 2023 github.com/fsgo  All Rights Reserved.
// Author: hidu <duv123@gmail.com>
// Date: 2023/10/8

package tcptunnel

import (
	"bytes"
	"io"
	"testing"

	"github.com/fsgo/fst"
)

func Test_rwWithToken(t *testing.T) {
	t.Run("no", func(t *testing.T) {
		bf := &bytes.Buffer{}
		b1 := &tb{bf: bf}
		w1 := rwWithToken(b1, "")
		_, e1 := w1.Write([]byte("hello"))
		fst.NoError(t, e1)
		fst.Equal(t, "hello", bf.String())
		content, _ := io.ReadAll(w1)
		fst.Equal(t, "hello", string(content))
	})
	t.Run("has", func(t *testing.T) {
		bf := &bytes.Buffer{}
		b1 := &tb{bf: bf}
		w1 := rwWithToken(b1, "hello-world")
		_, e1 := w1.Write([]byte("hello"))
		fst.NoError(t, e1)
		fst.NotEqual(t, "hello", bf.String())
		content, _ := io.ReadAll(w1)
		fst.Equal(t, "hello", string(content))
	})
}

var _ io.ReadWriteCloser = (*tb)(nil)

type tb struct {
	bf *bytes.Buffer
}

func (t *tb) Read(p []byte) (n int, err error) {
	return t.bf.Read(p)
}

func (t *tb) Write(p []byte) (n int, err error) {
	return t.bf.Write(p)
}

func (t *tb) Close() error {
	return nil
}
