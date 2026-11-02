package dashboard

import "strings"

// Custom model ids are the half of an address a client copies into its own config:
// "<node-prefix>/<id>". Slashes are allowed for namespaced models
// (e.g. "deepseek/deepseek-v4-1-flash" or "meta-llama/llama-3.1-70b-instruct").
const (
	// modelIDMaxLength keeps an id short enough to be readable in a config file.
	modelIDMaxLength = 200
	// modelIDSeparators are characters that should never be in a model ID.
	// Tab, newline, comma, semicolon are structural paste separators.
	modelIDSeparators = "\t\r\n,;"
	// modelIDReserved are the characters with a meaning in the kv key or syntax:
	// "|" separates key fields.
	modelIDReserved = `|"'` + "`" + `\<>`
)

// invalidModelIDReason reports why id cannot be stored as a custom model id, or "" when
// it can. The reason is written for the operator who pasted it, not for a log.
func invalidModelIDReason(id string) string {
	if id == "" {
		return "empty id"
	}
	if strings.ContainsAny(id, modelIDSeparators) {
		return "comma, semicolon, tab or newline cannot be part of an id"
	}
	if strings.TrimSpace(id) != id {
		return "an id cannot start or end with whitespace"
	}
	if strings.Contains(id, "  ") {
		return "an id cannot contain consecutive spaces"
	}
	if strings.ContainsAny(id, modelIDReserved) {
		return `an id cannot contain | " ' \ < > or a backtick`
	}
	if strings.HasPrefix(id, "/") || strings.HasSuffix(id, "/") {
		return "an id cannot start or end with a slash"
	}
	if strings.Contains(id, "//") {
		return "an id cannot contain consecutive slashes"
	}
	if len(id) > modelIDMaxLength {
		return "an id cannot be longer than 200 characters"
	}
	return ""
}
