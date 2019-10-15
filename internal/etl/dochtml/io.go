// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dochtml

import (
	"bytes"
	"io"
)

// limitBuffer is designed to apply a limit on the number of bytes
// that are allowed to be written to a *bytes.Buffer.
//
// As long as Remain is a non-negative value, writes to limitBuffer
// are passed through to the underlying buffer B, decreasing Remain
// by the number of bytes written.
// If more than Remain bytes have been attempted to be written to B,
// Remain becomes a negative value, and limitBuffer.Write starts to
// always return io.ErrShortWrite without writing to B.
type limitBuffer struct {
	B      *bytes.Buffer // Underlying buffer.
	Remain int64         // Until writes fail. Negative value means went beyond limit.
}

// Write implements io.Writer.
func (l *limitBuffer) Write(p []byte) (n int, err error) {
	if int64(len(p)) > l.Remain {
		l.Remain = -1
		return 0, io.ErrShortWrite
	}
	n, err = l.B.Write(p)
	l.Remain -= int64(n)
	return n, err
}
