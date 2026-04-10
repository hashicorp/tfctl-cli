// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package iostreams

import "io"

// Loud is unimpacted by quiet being set on the iostreams.
type Loud interface {
	IOStreams

	LoudErr() io.Writer
}

// LoudErr returns the writer to use for loud error output.
func (s *system) LoudErr() io.Writer {
	return s.err
}

// LoudErr returns the writer to use for loud error output in testing.
func (t *Testing) LoudErr() io.Writer {
	return t.Error
}

// UseLoud takes an IOStream and if it implements the Load interfaces, it will
// be used instead of the quiet alternatives.
func UseLoud(io IOStreams) IOStreams {
	l, ok := io.(Loud)
	if !ok {
		return io
	}

	return &loudWrap{
		IOStreams: l,
		l:         l,
	}
}

type loudWrap struct {
	IOStreams
	l Loud
}

// Err returns the loud error writer instead of the quiet one.
func (l *loudWrap) Err() io.Writer {
	return l.l.LoudErr()
}
