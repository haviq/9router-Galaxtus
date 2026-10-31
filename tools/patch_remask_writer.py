with open('internal/handlers/chat/model_remask.go', 'r', encoding='utf-8') as f:
    c = f.read()

remask_writer_code = '''
type modelRemaskWriter struct {
	http.ResponseWriter
	rule *db.ModelRemaskRule
}

// NewModelRemaskWriter wraps w to intercept both SSE chunks and non-streaming JSON
// bodies, ensuring the client receives the remasked alias and stripped CoT.
func NewModelRemaskWriter(w http.ResponseWriter, rule *db.ModelRemaskRule) http.ResponseWriter {
	if w == nil || rule == nil || !rule.Enabled {
		return w
	}
	return &modelRemaskWriter{
		ResponseWriter: w,
		rule:           rule,
	}
}

func (w *modelRemaskWriter) Write(p []byte) (int, error) {
	trimmed := bytes.TrimSpace(p)
	if len(trimmed) == 0 {
		return w.ResponseWriter.Write(p)
	}

	var masked []byte
	if bytes.HasPrefix(trimmed, []byte("data:")) {
		dataLine := trimmed[5:]
		if bytes.HasPrefix(dataLine, []byte(" ")) {
			dataLine = dataLine[1:]
		}
		if bytes.Equal(dataLine, []byte("[DONE]")) {
			return w.ResponseWriter.Write(p)
		}
		res := MaskSSEChunk(dataLine, w.rule)
		masked = append([]byte("data: "), res...)
		masked = append(masked, '\\n', '\\n')
	} else if bytes.HasPrefix(trimmed, []byte("{")) {
		masked = MaskResponseJSON(p, w.rule)
	} else {
		masked = p
	}

	if _, err := w.ResponseWriter.Write(masked); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (w *modelRemaskWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
'''

if 'NewModelRemaskWriter' not in c:
    c += remask_writer_code
    with open('internal/handlers/chat/model_remask.go', 'w', encoding='utf-8', newline='\n') as f:
        f.write(c)
    print("SUCCESS: model_remask.go patched")
else:
    print("ALREADY PATCHED: model_remask.go")

with open('internal/handlers/chat/chat.go', 'r', encoding='utf-8') as f:
    c2 = f.read()

target2 = '''	modelInfo, err := h.resolveModel(modelName)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}'''

replacement2 = '''	modelInfo, err := h.resolveModel(modelName)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if modelInfo != nil && modelInfo.RemaskRule != nil {
		w = NewModelRemaskWriter(w, modelInfo.RemaskRule)
	}'''

if target2 in c2 and 'w = NewModelRemaskWriter(w, modelInfo.RemaskRule)' not in c2:
    c2 = c2.replace(target2, replacement2, 1)
    with open('internal/handlers/chat/chat.go', 'w', encoding='utf-8', newline='\n') as f:
        f.write(c2)
    print("SUCCESS: chat.go patched")
else:
    print("ALREADY PATCHED or TARGET NOT FOUND: chat.go")

