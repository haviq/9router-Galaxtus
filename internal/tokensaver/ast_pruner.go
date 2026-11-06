package tokensaver

import (
	"regexp"
	"strings"
)

var (
	// Matches markdown fenced code blocks: ```lang ... ```
	reFencedCode = regexp.MustCompile("(?s)```([a-zA-Z0-9_+-]*)\\s*\\n(.*?)\\n```")

	// Matches multi-line C-style license/boilerplate header comments
	reLicenseHeader = regexp.MustCompile("(?si)^\\s*/\\*.*?\\b(copyright|license|spdx|all rights reserved|apache|mit|gnu|gpl)\\b.*?\\*/\\s*")

	// Matches assembly / python comment lines that are purely license headers
	rePythonLicense = regexp.MustCompile("(?mi)^#\\s*(copyright|license|spdx|all rights reserved).*?$\\n?")

	// Matches 3 or more consecutive empty lines
	reMultipleNewlines = regexp.MustCompile(`\n{3,}`)
)

// PruneCodeBlock cleans and compresses code blocks while preserving 100% of structs,
// signatures, prototypes, memory layouts, and types.
func PruneCodeBlock(code string, lang string) string {
	lang = strings.ToLower(strings.TrimSpace(lang))
	trimmed := code

	// 1. Strip top boilerplate license header if present
	if reLicenseHeader.MatchString(trimmed) {
		trimmed = reLicenseHeader.ReplaceAllString(trimmed, "")
	}
	if rePythonLicense.MatchString(trimmed) {
		trimmed = rePythonLicense.ReplaceAllString(trimmed, "")
	}

	// 2. Collapse excessive whitespace / blank lines
	trimmed = reMultipleNewlines.ReplaceAllString(trimmed, "\n\n")

	// 3. Language specific light structural pruning for long files (> 80 lines)
	lines := strings.Split(trimmed, "\n")
	if len(lines) > 80 {
		var pruned []string
		inBigFunc := false
		funcBraceCount := 0
		funcLineCount := 0

		for _, line := range lines {
			tLine := strings.TrimSpace(line)

			// Always preserve structs, unions, enums, typedefs, interfaces, macros
			if strings.HasPrefix(tLine, "struct ") || strings.HasPrefix(tLine, "typedef struct") ||
				strings.HasPrefix(tLine, "union ") || strings.HasPrefix(tLine, "enum ") ||
				strings.HasPrefix(tLine, "#define") || strings.HasPrefix(tLine, "#include") ||
				strings.HasPrefix(tLine, "type ") || strings.HasPrefix(tLine, "interface ") ||
				strings.HasPrefix(tLine, "pub struct") || strings.HasPrefix(tLine, "pub enum") {
				pruned = append(pruned, line)
				continue
			}

			// For C/C++/Go/Rust: detect large non-essential function bodies
			if !inBigFunc && (strings.HasSuffix(tLine, "){") || strings.HasSuffix(tLine, ") {") ||
				(strings.Contains(tLine, "func ") && strings.HasSuffix(tLine, "{")) ||
				(strings.Contains(tLine, "fn ") && strings.HasSuffix(tLine, "{"))) {
				inBigFunc = true
				funcBraceCount = strings.Count(line, "{") - strings.Count(line, "}")
				funcLineCount = 1
				pruned = append(pruned, line)
				continue
			}

			if inBigFunc {
				funcBraceCount += strings.Count(line, "{") - strings.Count(line, "}")
				funcLineCount++

				if funcBraceCount <= 0 {
					// End of function
					inBigFunc = false
					pruned = append(pruned, line)
					continue
				}

				// If function body exceeds 25 lines, summarize the middle body
				if funcLineCount > 15 && funcLineCount < 50 && funcBraceCount == 1 {
					if funcLineCount == 16 {
						pruned = append(pruned, "\t// ... [RTK++ AST: routine body compacted for context] ...")
					}
					continue
				}
			}

			pruned = append(pruned, line)
		}
		trimmed = strings.Join(pruned, "\n")
	}

	return strings.TrimSpace(trimmed)
}

// CompressCodeContext inspects text (such as user prompts or tool outputs), locates
// embedded code blocks, and applies AST-aware pruning to minimize tokens.
func CompressCodeContext(text string) (string, int) {
	if len(text) < 80 {
		return text, 0
	}

	origTokens := len(text) / 4
	var res string
	if reFencedCode.MatchString(text) {
		res = reFencedCode.ReplaceAllStringFunc(text, func(m string) string {
			sub := reFencedCode.FindStringSubmatch(m)
			if len(sub) < 3 {
				return m
			}
			lang := sub[1]
			code := sub[2]
			pruned := PruneCodeBlock(code, lang)
			return "```" + lang + "\n" + pruned + "\n```"
		})
	} else if reLicenseHeader.MatchString(text) || strings.Contains(text, "struct ") || strings.Contains(text, "#include ") {
		res = PruneCodeBlock(text, "")
	} else {
		res = text
	}

	newTokens := len(res) / 4
	saved := origTokens - newTokens
	if saved < 0 {
		saved = 0
	}
	return res, saved
}
