package chat

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBufferedResponseWriter(t *testing.T) {
	rec := httptest.NewRecorder()
	buf := &bytes.Buffer{}
	bw := newBufferedResponseWriter(rec, buf)

	bw.Header().Set("Content-Type", "application/json")
	bw.WriteHeader(http.StatusOK)
	_, err := bw.Write([]byte(`{"choices":[]}`))
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}

	if buf.String() != `{"choices":[]}` {
		t.Errorf("buffer mismatch: %s", buf.String())
	}

	flushBufferedResponseWriter(rec, bw, buf)
	if rec.Body.String() != `{"choices":[]}` {
		t.Errorf("recorder body mismatch: %s", rec.Body.String())
	}
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Errorf("recorder header mismatch")
	}
}

