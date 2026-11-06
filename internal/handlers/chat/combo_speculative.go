package chat

import (
	json "9router/proxy/internal/fastjson"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"9router/proxy/internal/log"
	"9router/proxy/internal/providers"
	internalproxy "9router/proxy/internal/proxy"
)

const defaultSpeculativeFastTimeout = 2500 * time.Millisecond

type speculativePipeWriter struct {
	mu         sync.Mutex
	cond       *sync.Cond
	header     http.Header
	code       int
	initialBuf bytes.Buffer
	firstReady chan struct{}
	firstOnce  sync.Once
	chunks     [][]byte
	isClosed   bool
	err        error
}

func newSpeculativePipeWriter() *speculativePipeWriter {
	p := &speculativePipeWriter{
		header:     make(http.Header),
		firstReady: make(chan struct{}),
	}
	p.cond = sync.NewCond(&p.mu)
	return p
}

func (p *speculativePipeWriter) Header() http.Header {
	return p.header
}

func (p *speculativePipeWriter) WriteHeader(code int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.code == 0 {
		p.code = code
	}
}

func (p *speculativePipeWriter) Write(data []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.isClosed {
		return 0, io.ErrClosedPipe
	}
	if p.code == 0 {
		p.code = http.StatusOK
	}
	cp := make([]byte, len(data))
	copy(cp, data)

	if p.initialBuf.Len() < 1024 {
		p.initialBuf.Write(cp)
	}

	p.chunks = append(p.chunks, cp)
	p.cond.Broadcast()

	bufBytes := p.initialBuf.Bytes()
	if internalproxy.DetectSSERefusal(bufBytes) || internalproxy.HasSSEContent(cp) || p.initialBuf.Len() >= 512 {
		p.firstOnce.Do(func() {
			close(p.firstReady)
		})
	}
	return len(data), nil
}

func (p *speculativePipeWriter) Flush() {
	p.mu.Lock()
	defer p.mu.Unlock()
	bufBytes := p.initialBuf.Bytes()
	if internalproxy.DetectSSERefusal(bufBytes) || internalproxy.HasSSEContent(bufBytes) {
		p.firstOnce.Do(func() {
			close(p.firstReady)
		})
	}
}

func (p *speculativePipeWriter) Close(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.isClosed {
		p.isClosed = true
		p.err = err
		p.firstOnce.Do(func() {
			close(p.firstReady)
		})
		p.cond.Broadcast()
	}
}

func (h *ChatHandler) forwardSingleCandidate(ctx context.Context, w http.ResponseWriter, entry string, body []byte, isStream bool, translateResponse bool, comboName string, endpoint string) error {
	modelInfo := h.resolveModelEntry(entry)
	if modelInfo == nil {
		return fmt.Errorf("unresolved model: %s", entry)
	}

	var connID string
	var connName, connEmail string
	var connData *ConnectionData
	var picked *ProviderConnection

	if cfg, ok := providers.KnownProviders[modelInfo.Provider]; ok && (cfg.NoAuth || cfg.DefaultAPIKey != "") {
		connData = &ConnectionData{
			APIKey:      cfg.DefaultAPIKey,
			ProxyPoolID: h.ResolveProviderProxyPoolID(modelInfo.Provider),
		}
	} else {
		var err error
		picked, connData, err = h.getBestConnectionWithContext(ctx, modelInfo.Provider, modelInfo.ConnectionID, nil, modelInfo.Model)
		if err != nil {
			return fmt.Errorf("no connection for %s/%s: %w", modelInfo.Provider, modelInfo.Model, err)
		}
		connID = picked.ID
		connName = connObjName(picked)
		connEmail = connObjEmail(picked)
	}

	var upstreamBody map[string]any
	if err := json.Unmarshal(body, &upstreamBody); err != nil {
		return fmt.Errorf("unmarshal body: %w", err)
	}
	upstreamBody["model"] = h.wireModelForNode(modelInfo.Provider, modelInfo.Model)
	repairToolCallIDsInMap(upstreamBody)
	ctx = h.applyJailbreak(ctx, upstreamBody, comboName, jailbreakLegIDs(modelInfo.Provider, modelInfo.Model)...)
	upstreamJSON, err := json.Marshal(upstreamBody)
	if err != nil {
		return fmt.Errorf("marshal body: %w", err)
	}

	if endpoint == "" {
		endpoint = forwardEndpoint(ctx, "/v1/chat/completions")
	}

	return h.tryForwardWithConnection(forwardRequestParams{
		Ctx:               ctx,
		W:                 w,
		Provider:          modelInfo.Provider,
		Model:             modelInfo.Model,
		RequestedModel:    entry,
		ComboName:         comboName,
		ConnectionID:      connID,
		ConnName:          connName,
		ConnEmail:         connEmail,
		ConnData:          connData,
		Body:              upstreamJSON,
		IsStream:          isStream,
		TranslateResponse: translateResponse,
		Endpoint:          endpoint,
	})
}

// handleSpeculative executes a fast model concurrently with a reasoner model.
// If the fast model begins outputting valid non-refusal tokens within the speculative window,
// it wins early and the reasoner model's upstream connection is canceled.
// If the fast model rejects or fails, the reasoner model takes over seamlessly without
// streaming refusal tokens to the client.
func (h *ChatHandler) handleSpeculative(ctx context.Context, w http.ResponseWriter, body []byte, models []string, isStream bool, translateResponse bool, comboName string) {
	if len(models) < 2 {
		h.handleComboFallback(ctx, w, body, models, "fallback", isStream, translateResponse, comboName, 0)
		return
	}

	fastEntry := models[0]
	heavyEntry := models[1]

	fastCtx, fastCancel := context.WithCancel(ctx)
	heavyCtx, heavyCancel := context.WithCancel(ctx)
	defer fastCancel()
	defer heavyCancel()

	if isStream {
		fastPipe := newSpeculativePipeWriter()
		heavyPipe := newSpeculativePipeWriter()

		go func() {
			err := h.forwardSingleCandidate(fastCtx, fastPipe, fastEntry, body, true, translateResponse, comboName, "")
			fastPipe.Close(err)
		}()

		go func() {
			err := h.forwardSingleCandidate(heavyCtx, heavyPipe, heavyEntry, body, true, translateResponse, comboName, "")
			heavyPipe.Close(err)
		}()

		select {
		case <-fastPipe.firstReady:
			fastBytes := fastPipe.initialBuf.Bytes()
			isRefusal := internalproxy.DetectSSERefusal(fastBytes)
			if fastPipe.code >= 400 || isRefusal || fastPipe.err != nil {
				log.Warn("combo", "speculative fast-path rejected; switching to heavy", "model", fastEntry, "refusal", isRefusal, "code", fastPipe.code)
				fastCancel()
				streamPipeToClient(ctx, heavyPipe, w)
				return
			}
			log.Info("combo", "speculative fast-path committed early", "fast_model", fastEntry)
			heavyCancel()
			streamPipeToClient(ctx, fastPipe, w)
			return

		case <-heavyPipe.firstReady:
			fastPipe.mu.Lock()
			fastClosed := fastPipe.isClosed
			fastCode := fastPipe.code
			fastErr := fastPipe.err
			fastBytes := fastPipe.initialBuf.Bytes()
			fastPipe.mu.Unlock()

			if fastClosed && (fastCode >= 400 || fastErr != nil || internalproxy.DetectSSERefusal(fastBytes)) {
				fastCancel()
				streamPipeToClient(ctx, heavyPipe, w)
				return
			}

			// Brief grace period for fast path to commit
			select {
			case <-fastPipe.firstReady:
				if fastPipe.code < 400 && !internalproxy.DetectSSERefusal(fastPipe.initialBuf.Bytes()) && fastPipe.err == nil {
					heavyCancel()
					streamPipeToClient(ctx, fastPipe, w)
					return
				}
				fastCancel()
				streamPipeToClient(ctx, heavyPipe, w)
				return
			case <-time.After(400 * time.Millisecond):
				fastCancel()
				streamPipeToClient(ctx, heavyPipe, w)
				return
			case <-ctx.Done():
				return
			}

		case <-time.After(defaultSpeculativeFastTimeout):
			log.Warn("combo", "speculative fast-path timeout; committing to heavy-path", "fast", fastEntry, "heavy", heavyEntry)
			fastCancel()
			streamPipeToClient(ctx, heavyPipe, w)
			return

		case <-ctx.Done():
			return
		}
	} else {
		// Non-streaming speculative execution
		fastBuf := &responseBuffer{header: http.Header{}}
		heavyBuf := &responseBuffer{header: http.Header{}}

		fastDone := make(chan error, 1)
		heavyDone := make(chan error, 1)

		go func() {
			fastDone <- h.forwardSingleCandidate(fastCtx, fastBuf, fastEntry, body, false, translateResponse, comboName, "")
		}()

		go func() {
			heavyDone <- h.forwardSingleCandidate(heavyCtx, heavyBuf, heavyEntry, body, false, translateResponse, comboName, "")
		}()

		select {
		case err := <-fastDone:
			if err == nil && fastBuf.code < 400 && !internalproxy.DetectSSERefusal(fastBuf.body.Bytes()) {
				heavyCancel()
				writeResponseBuffer(w, fastBuf)
				return
			}
			log.Warn("combo", "speculative fast-path failed non-streaming; switching to heavy", "model", fastEntry, "err", err)
			fastCancel()
			select {
			case err2 := <-heavyDone:
				if err2 == nil {
					writeResponseBuffer(w, heavyBuf)
					return
				}
				if len(models) > 2 {
					h.handleComboFallback(ctx, w, body, models[2:], "fallback", false, translateResponse, comboName, 0)
					return
				}
				writeResponseBuffer(w, heavyBuf)
				return
			case <-ctx.Done():
				return
			}

		case err2 := <-heavyDone:
			if err2 == nil && heavyBuf.code < 400 {
				fastCancel()
				writeResponseBuffer(w, heavyBuf)
				return
			}

		case <-time.After(defaultSpeculativeFastTimeout):
			fastCancel()
			select {
			case err2 := <-heavyDone:
				if err2 == nil {
					writeResponseBuffer(w, heavyBuf)
					return
				}
				if len(models) > 2 {
					h.handleComboFallback(ctx, w, body, models[2:], "fallback", false, translateResponse, comboName, 0)
					return
				}
				writeResponseBuffer(w, heavyBuf)
				return
			case <-ctx.Done():
				return
			}

		case <-ctx.Done():
			return
		}
	}
}

func streamPipeToClient(ctx context.Context, pipe *speculativePipeWriter, w http.ResponseWriter) {
	doneMonitor := make(chan struct{})
	defer close(doneMonitor)
	go func() {
		select {
		case <-ctx.Done():
			pipe.Close(ctx.Err())
		case <-doneMonitor:
		}
	}()

	pipe.mu.Lock()
	for k, v := range pipe.header {
		for _, val := range v {
			w.Header().Add(k, val)
		}
	}
	code := pipe.code
	if code == 0 {
		code = http.StatusOK
	}
	w.WriteHeader(code)
	flusher, hasFlush := w.(http.Flusher)
	if hasFlush {
		flusher.Flush()
	}

	readIdx := 0
	for {
		for readIdx < len(pipe.chunks) {
			chunk := pipe.chunks[readIdx]
			readIdx++
			pipe.mu.Unlock()
			w.Write(chunk)
			if hasFlush {
				flusher.Flush()
			}
			pipe.mu.Lock()
		}
		if pipe.isClosed {
			pipe.mu.Unlock()
			return
		}
		pipe.cond.Wait()
	}
}

func writeResponseBuffer(w http.ResponseWriter, buf *responseBuffer) {
	for k, v := range buf.header {
		for _, val := range v {
			w.Header().Add(k, val)
		}
	}
	code := buf.code
	if code == 0 {
		code = http.StatusOK
	}
	w.WriteHeader(code)
	if buf.body.Len() > 0 {
		w.Write(buf.body.Bytes())
	}
}
