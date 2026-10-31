package chat

import (
	"context"

	"9router/proxy/internal/db"
	"9router/proxy/internal/log"
	"9router/proxy/internal/translator"
)

// resolveJailbreak returns the injection document for a request, or nil when
// nothing should be injected. ids are the candidate model ids in specificity
// order — the id the client sent first, then the ids the request is forwarded
// to — and the resolver applies exact bindings, keyword rules, and the
// exclude lists (see internal/db/jailbreak_keywords.go for the precedence).
// An empty document means pass-through, exactly like a toggle left off.
func (h *ChatHandler) resolveJailbreak(ids ...string) *db.JailbreakDoc {
	if h.Repo == nil {
		return nil
	}
	doc, err := h.Repo.ResolveJailbreakDoc(ids...)
	if err != nil {
		log.Warn("jailbreak", "doc lookup failed", "ids", ids, "error", err)
		return nil
	}
	if doc == nil || doc.Prompt == "" {
		return nil
	}
	return doc
}

// applyJailbreak mutates the outbound request body for a request that resolves
// to a document and returns a context carrying the Gemini-safety flag for the
// forward chain. Called before the body is marshalled, so the prompt-cache hash
// always reflects the injected prompt. Every forward path calls this: single
// chat, Claude single, combo fallback, Claude combo, fusion panel, fusion
// judge. requestedModel is the id the client sent — a model id, or the combo
// name, which arms every leg of that combo. legIDs are the ids the request is
// actually forwarded to, so a keyword such as "deepseek" covers a combo leg
// like providerx/deepseek-4-pro without arming that id.
func (h *ChatHandler) applyJailbreak(ctx context.Context, body map[string]any, requestedModel string, legIDs ...string) context.Context {
	doc := h.resolveJailbreak(append([]string{requestedModel}, legIDs...)...)
	if doc == nil {
		return ctx
	}
	applySystemPrompt(body, doc.Prompt, doc.InjectMode)
	log.Info("jailbreak", "applied", "model", requestedModel,
		"prompt_bytes", len(doc.Prompt), "safety_off", doc.GeminiSafety, "mode", injectModeOrDefault(doc.InjectMode))
	// Best-effort usage counter for the dashboard's per-preset hit count. A
	// failed write must not fail a request that is already being injected.
	if err := h.Repo.RecordJailbreakUsage(doc.ID); err != nil {
		log.Warn("jailbreak", "usage counter failed", "preset", doc.ID, "error", err)
	}
	if doc.GeminiSafety {
		ctx = translator.WithGeminiSafety(ctx, true)
	}
	return ctx
}

// jailbreakLegIDs returns the two ids a forwarded leg answers to: the
// "provider/model" form the dashboard and /v1/models use, and the bare model
// the upstream body carries. A keyword or exclude matches either.
func jailbreakLegIDs(provider, model string) []string {
	return []string{provider + "/" + model, model}
}

// injectModeOrDefault maps an unset mode to prepend, so a preset written by an
// older build is applied the way it always was.
func injectModeOrDefault(mode string) string {
	if mode == "" {
		return db.JailbreakInjectPrepend
	}
	return mode
}

// applySystemPrompt injects the configured prompt into the outbound body using
// the preset's inject mode. Two wire formats reach here: OpenAI (system role in
// "messages") and Claude (top-level "system", string or block array).
//
//	prepend  the prompt goes ahead of the existing system content
//	append   the prompt goes after the existing system content
//	replace  the existing system content is dropped
//
// A body with no system content at all (a bare "messages" list) is treated as
// empty, so append and replace still land the prompt as the leading system
// message rather than dropping it.
func applySystemPrompt(body map[string]any, prompt string, mode string) {
	switch injectModeOrDefault(mode) {
	case db.JailbreakInjectAppend:
		appendSystemPrompt(body, prompt)
	case db.JailbreakInjectReplace:
		replaceSystemPrompt(body, prompt)
	default:
		prependSystemPrompt(body, prompt)
	}
}

// prependSystemPrompt injects the prompt ahead of the original content,
// original content intact after it.
func prependSystemPrompt(body map[string]any, prompt string) {
	if setSystemString(body, prompt+"\n\n") {
		return
	}
	if blocks, ok := body["system"].([]any); ok {
		body["system"] = append([]any{map[string]any{"type": "text", "text": prompt}}, blocks...)
		return
	}
	prependSystemMessage(body, prompt)
}

// appendSystemPrompt injects the prompt after the original system content, so a
// preset can refine an operator instruction instead of leading it.
func appendSystemPrompt(body map[string]any, prompt string) {
	if s, ok := body["system"].(string); ok {
		body["system"] = s + "\n\n" + prompt
		return
	}
	if blocks, ok := body["system"].([]any); ok {
		body["system"] = append(blocks, map[string]any{"type": "text", "text": prompt})
		return
	}
	prependSystemMessage(body, prompt)
}

// replaceSystemPrompt drops any existing system content and sends only the
// preset prompt.
func replaceSystemPrompt(body map[string]any, prompt string) {
	if _, ok := body["system"]; ok {
		body["system"] = prompt
		return
	}
	prependSystemMessage(body, prompt)
}

// setSystemString rewrites a string system field with prefix and reports
// whether it did. A non-string, non-block system is left for the caller's
// message fallback.
func setSystemString(body map[string]any, prefix string) bool {
	s, ok := body["system"].(string)
	if !ok {
		return false
	}
	body["system"] = prefix + s
	return true
}

// prependSystemMessage adds the prompt as the first system message of an
// OpenAI-style body, preserving the original messages after it.
func prependSystemMessage(body map[string]any, prompt string) {
	msgs, _ := body["messages"].([]any)
	body["messages"] = append([]any{map[string]any{"role": "system", "content": prompt}}, msgs...)
}
