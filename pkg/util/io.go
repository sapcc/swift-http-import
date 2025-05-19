// SPDX-FileCopyrightText: 2018 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package util

import "io"

// FullReader is an io.ReadCloser whose Read() implementation always fills the read
// buffer as much as possible by calling Base.Read() repeatedly.
type FullReader struct {
	Base io.ReadCloser
}

// Read implements the io.Reader interface.
func (r *FullReader) Read(buf []byte) (int, error) {
	numRead := 0
	for numRead < len(buf) {
		n, err := r.Base.Read(buf[numRead:])
		numRead += n
		if err != nil { // including io.EOF
			return numRead, err
		}
	}
	return numRead, nil
}

// Close implements the io.Reader interface.
func (r *FullReader) Close() error {
	return r.Base.Close()
}
