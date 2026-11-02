package chat

import (
	"bytes"
	"net/http"
)

type bufferedResponseWriter struct {
	underlying http.ResponseWriter
	header     http.Header
	buf        *bytes.Buffer
	statusCode int
}

func newBufferedResponseWriter(underlying http.ResponseWriter, buf *bytes.Buffer) *bufferedResponseWriter {
	return &bufferedResponseWriter{
		underlying: underlying,
		header:     make(http.Header),
		buf:        buf,
		statusCode: http.StatusOK,
	}
}

func (b *bufferedResponseWriter) Header() http.Header {
	return b.header
}

func (b *bufferedResponseWriter) Write(p []byte) (int, error) {
	return b.buf.Write(p)
}

func (b *bufferedResponseWriter) WriteHeader(statusCode int) {
	b.statusCode = statusCode
}

func (b *bufferedResponseWriter) Flush() {}

func flushBufferedResponseWriter(dest http.ResponseWriter, b *bufferedResponseWriter, buf *bytes.Buffer) {
	if b == nil || buf == nil {
		return
	}
	for k, vv := range b.header {
		for _, v := range vv {
			dest.Header().Add(k, v)
		}
	}
	if b.statusCode != 0 {
		dest.WriteHeader(b.statusCode)
	}
	_, _ = dest.Write(buf.Bytes())
}

