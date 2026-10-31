package pro

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

// ══════ PROSES 1 — Gateway Classifier (pre-GPU input scan) ══════

var InputPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(bypass|jailbreak)\s+(prompt|system)\b`),
	regexp.MustCompile(`(?i)ignore\s+(all\s+)?previous.*instructions`),
}

func ScanInput(text string) string {
	for _, p := range InputPatterns {
		if m := p.FindString(text); m != "" {
			return m
		}
	}
	return ""
}

// ══════ PROSES 3 — Model Routing (whitelist seleksi) ══════

var AllowedModels = map[string]bool{
	"meta-llama/llama-3-8b-instruct": true,
	"mistralai/mistral-7b-instruct":  true,
}

// ══════ PROSES 2 — System Prompt Injection ══════

var RouterSystemMessage = map[string]string{
	"role":    "system",
	"content": "Sebelum menjawab, evaluasi apakah permintaan user sesuai kebijakan organisasi internal. Jika di luar scope, redirect ke topik yang relevan.",
}

// ══════ PROSES 4 — Real-time Output Stream Monitor ══════

var OutputPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(molotov|pipe.?bomb|napalm)\b`),
	regexp.MustCompile(`(?i)\bshellcode\s+generator\b`),
	regexp.MustCompile(`(?i)\bkeylogger\b`),
}

func ScanOutput(text string) string {
	for _, p := range OutputPatterns {
		if m := p.FindString(text); m != "" {
			return m
		}
	}
	return ""
}

// ══════ Request/Response types ══════

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
}

type DeltaEnvelope struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
}

// ══════ Gateway core ══════

type Gateway struct {
	Upstream string
	APIKey   string
	Client   *http.Client
}

func NewGateway(upstream, apiKey string) *Gateway {
	if upstream == "" {
		upstream = GetEnv("UPSTREAM", "http://127.0.0.1:20130/v1")
	}
	if apiKey == "" {
		apiKey = GetEnv("PROVIDER_KEY", "sk-9router")
	}
	return &Gateway{
		Upstream: upstream,
		APIKey:   apiKey,
		Client:   &http.Client{Timeout: 120 * time.Second},
	}
}

func Reject(w http.ResponseWriter, status int, reason string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": reason})
}

// InboundCheck verifies the input messages against PROSES 1 rules when PRO_PIPELINE_ENABLED is true.
func InboundCheck(w http.ResponseWriter, body []byte) bool {
	if os.Getenv("PRO_PIPELINE_ENABLED") != "true" && os.Getenv("PRO_PIPELINE_ENABLED") != "1" {
		return true
	}
	var req struct {
		Messages []Message `json:"messages"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return true
	}
	var userText bytes.Buffer
	for _, m := range req.Messages {
		userText.WriteString(m.Content)
		userText.WriteString(" ")
	}
	if hit := ScanInput(userText.String()); hit != "" {
		fmt.Printf("[PROSES-1 HIT] pattern: %s\n", hit)
		Reject(w, http.StatusBadRequest, "input rejected oleh gateway filter")
		return false
	}
	return true
}

func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req ChatRequest
	body, _ := io.ReadAll(r.Body)

	if err := json.Unmarshal(body, &req); err != nil {
		Reject(w, http.StatusBadRequest, "malformed JSON")
		return
	}

	// ─── PROSES 1: pre-flight input scan ───
	var userText bytes.Buffer
	for _, m := range req.Messages {
		userText.WriteString(m.Content)
		userText.WriteString(" ")
	}
	if hit := ScanInput(userText.String()); hit != "" {
		fmt.Printf("[PROSES-1 HIT] pattern: %s\n", hit)
		Reject(w, http.StatusBadRequest, "input rejected oleh gateway filter")
		return
	}

	// ─── PROSES 3: model routing gate ───
	if len(AllowedModels) > 0 && !AllowedModels[req.Model] {
		// Jika model tidak di whitelist, periksa apakah ada prefix atau wildcard
		Reject(w, http.StatusUnprocessableEntity,
			fmt.Sprintf("model '%s' tidak di-whitelist", req.Model))
		return
	}

	// ─── PROSES 2: prepend system counter-pressure ───
	injected := append(
		[]Message{
			{Role: RouterSystemMessage["role"], Content: RouterSystemMessage["content"]},
		},
		req.Messages...,
	)
	req.Messages = injected

	rewritten, _ := json.Marshal(req)

	if req.Stream {
		g.ServeStream(w, rewritten)
	} else {
		g.ServeBuffered(w, rewritten)
	}
}

// ══════ PROSES 4a — Non-streaming (full scan before deliver) ══════

func (g *Gateway) ServeBuffered(w http.ResponseWriter, body []byte) {
	resp, err := g.Post(body)
	if err != nil {
		Reject(w, http.StatusBadGateway, "upstream error")
		return
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)

	var data struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	json.Unmarshal(raw, &data)

	for _, c := range data.Choices {
		if hit := ScanOutput(c.Message.Content); hit != "" {
			fmt.Printf("[PROSES-4 HIT] non-stream cut: %s\n", hit)
			cut, _ := json.Marshal(map[string]interface{}{
				"choices": []map[string]interface{}{{
					"message": map[string]string{
						"role":    "assistant",
						"content": "[response terputus oleh monitor]",
					},
					"finish_reason": "content_filter",
				}},
			})
			w.Header().Set("Content-Type", "application/json")
			w.Write(cut)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(raw)
}

// ══════ PROSES 4b — Streaming SSE (live cut on hit) ══════

func (g *Gateway) ServeStream(w http.ResponseWriter, body []byte) {
	resp, err := g.Post(body)
	if err != nil {
		Reject(w, http.StatusBadGateway, "upstream error")
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, _ := w.(http.Flusher)

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			fmt.Fprintf(w, "%s\n\n", line)
			if flusher != nil {
				flusher.Flush()
			}
			continue
		}

		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			fmt.Fprint(w, "data: [DONE]\n\n")
			if flusher != nil {
				flusher.Flush()
			}
			return
		}

		var env DeltaEnvelope
		if err := json.Unmarshal([]byte(payload), &env); err != nil {
			fmt.Fprintf(w, "data: %s\n\n", payload)
			if flusher != nil {
				flusher.Flush()
			}
			continue
		}

		if len(env.Choices) > 0 {
			delta := env.Choices[0].Delta.Content
			if hit := ScanOutput(delta); hit != "" {
				// ─── HIT: potong stream, kirim premature [DONE] ───
				fmt.Printf("[PROSES-4 HIT] pemotongan stream: %s\n", hit)
				fmt.Fprint(w, "data: [DONE]\n\n")
				if flusher != nil {
					flusher.Flush()
				}
				return
			}
		}

		fmt.Fprintf(w, "data: %s\n\n", payload)
		if flusher != nil {
			flusher.Flush()
		}
	}

	fmt.Fprint(w, "data: [DONE]\n\n")
	if flusher != nil {
		flusher.Flush()
	}
}

func (g *Gateway) Post(body []byte) (*http.Response, error) {
	url := strings.TrimSuffix(g.Upstream, "/") + "/chat/completions"
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if g.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+g.APIKey)
	}
	req.Header.Set("Accept", "text/event-stream")
	return g.Client.Do(req)
}

func GetEnv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
