// Copyright(C) 2023 github.com/fsgo  All Rights Reserved.
// Author: hidu <duv123@gmail.com>
// Date: 2023/10/8

package internal

import "io"

func RWCopy(in io.ReadWriteCloser, out io.ReadWriteCloser) error {
	defer in.Close()
	defer out.Close()
	ec := make(chan error, 2)
	go func() {
		_, err := io.Copy(in, out)
		ec <- err
	}()
	go func() {
		_, err := io.Copy(out, in)
		ec <- err
	}()
	return <-ec
}
