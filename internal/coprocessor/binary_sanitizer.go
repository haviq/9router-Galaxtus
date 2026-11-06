package coprocessor

import (
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Common bad-byte presets for offensive exploitation primitives.
var (
	BadBytesNullOnly       = []byte{0x00}
	BadBytesStringSafe     = []byte{0x00, 0x0A, 0x0D}
	BadBytesHTTPMethodSafe = []byte{0x00, 0x0A, 0x0D, 0x20, 0x3F, 0x26}
)

var (
	reEscapedHex = regexp.MustCompile(`\\x([0-9a-fA-F]{2})`)
	reCArrayHex  = regexp.MustCompile(`0x([0-9a-fA-F]{2})`)
	reRawHexWord = regexp.MustCompile(`\b([0-9a-fA-F]{2})\b`)
)

// BadByteHit records an occurrence of a prohibited byte within a payload.
type BadByteHit struct {
	Offset int    `json:"offset"`
	Byte   string `json:"byte"`
}

// ShellcodeScanResult summarizes the analysis of extracted machine code.
type ShellcodeScanResult struct {
	ByteLength int          `json:"byte_length"`
	BadHits    []BadByteHit `json:"bad_hits"`
	IsClean    bool         `json:"is_clean"`
}

// ExtractShellcodeBytes parses machine code bytes from raw text, escape sequences (\x..), or C array syntax.
func ExtractShellcodeBytes(input string) []byte {
	// 1. Try \xNN escape sequences
	matches := reEscapedHex.FindAllStringSubmatch(input, -1)
	if len(matches) > 0 {
		out := make([]byte, len(matches))
		for i, m := range matches {
			b, _ := hex.DecodeString(m[1])
			if len(b) > 0 {
				out[i] = b[0]
			}
		}
		return out
	}

	// 2. Try 0xNN C-array initializers
	matches = reCArrayHex.FindAllStringSubmatch(input, -1)
	if len(matches) > 0 {
		out := make([]byte, len(matches))
		for i, m := range matches {
			b, _ := hex.DecodeString(m[1])
			if len(b) > 0 {
				out[i] = b[0]
			}
		}
		return out
	}

	// 3. Try raw hex sequences (e.g. 48 31 c0)
	cleaned := strings.Map(func(r rune) rune {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F') {
			return r
		}
		return -1
	}, input)

	if len(cleaned) >= 2 && len(cleaned)%2 == 0 {
		if raw, err := hex.DecodeString(cleaned); err == nil && len(raw) > 0 {
			return raw
		}
	}

	return nil
}

// ScanBadBytes evaluates machine code against a set of prohibited bad bytes.
func ScanBadBytes(shellcode []byte, badBytes []byte) ShellcodeScanResult {
	if len(shellcode) == 0 {
		return ShellcodeScanResult{ByteLength: 0, BadHits: nil, IsClean: true}
	}

	badSet := make(map[byte]bool, len(badBytes))
	for _, b := range badBytes {
		badSet[b] = true
	}

	var hits []BadByteHit
	for idx, b := range shellcode {
		if badSet[b] {
			hits = append(hits, BadByteHit{
				Offset: idx,
				Byte:   fmt.Sprintf("\\x%02X", b),
			})
		}
	}

	return ShellcodeScanResult{
		ByteLength: len(shellcode),
		BadHits:    hits,
		IsClean:    len(hits) == 0,
	}
}

// LintIssue represents an architectural or ABI violation in generated code.
type LintIssue struct {
	Severity    string `json:"severity"` // "ERROR", "WARNING", "INFO"
	Line        int    `json:"line"`
	Instruction string `json:"instruction"`
	Message     string `json:"message"`
}

// LintX64Assembly scans x86-64 assembly instructions for common model hallucinations and ABI bugs:
// - Memory-to-memory operations (e.g. mov [rax], [rbx]) which are invalid on x86/x64
// - Stack alignment violations before call (System V & Windows x64 require 16-byte RSP alignment)
// - Windows x64 missing 32-byte shadow space before external call
func LintX64Assembly(asmText string, targetOS string) []LintIssue {
	var issues []LintIssue
	lines := strings.Split(asmText, "\n")

	rspOffset := 0 // Tracks cumulative stack pointer delta
	isWindows := strings.EqualFold(targetOS, "windows")

	for i, rawLine := range lines {
		lineNum := i + 1
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}

		lower := strings.ToLower(line)

		// 1. Detect memory-to-memory MOV/ADD/SUB (e.g., mov [rax], [rbx] or mov qword ptr [rax], qword ptr [rbx])
		if strings.HasPrefix(lower, "mov ") || strings.HasPrefix(lower, "add ") || strings.HasPrefix(lower, "sub ") {
			parts := strings.SplitN(lower[4:], ",", 2)
			if len(parts) == 2 {
				dst := strings.TrimSpace(parts[0])
				src := strings.TrimSpace(parts[1])
				if strings.Contains(dst, "[") && strings.Contains(src, "[") {
					issues = append(issues, LintIssue{
						Severity:    "ERROR",
						Line:        lineNum,
						Instruction: line,
						Message:     "x86/x64 architecture does not support memory-to-memory operations in a single instruction.",
					})
				}
			}
		}

		// 2. Track stack mutations
		if strings.HasPrefix(lower, "push ") {
			rspOffset -= 8
		} else if strings.HasPrefix(lower, "pop ") {
			rspOffset += 8
		} else if strings.HasPrefix(lower, "sub rsp,") || strings.HasPrefix(lower, "sub esp,") {
			parts := strings.Split(lower, ",")
			if len(parts) == 2 {
				valStr := strings.TrimSpace(parts[1])
				valStr = strings.TrimPrefix(valStr, "0x")
				if n, err := strconv.ParseInt(valStr, 16, 64); err == nil {
					rspOffset -= int(n)
				} else if n, err := strconv.ParseInt(valStr, 10, 64); err == nil {
					rspOffset -= int(n)
				}
			}
		} else if strings.HasPrefix(lower, "add rsp,") || strings.HasPrefix(lower, "add esp,") {
			parts := strings.Split(lower, ",")
			if len(parts) == 2 {
				valStr := strings.TrimSpace(parts[1])
				valStr = strings.TrimPrefix(valStr, "0x")
				if n, err := strconv.ParseInt(valStr, 16, 64); err == nil {
					rspOffset += int(n)
				} else if n, err := strconv.ParseInt(valStr, 10, 64); err == nil {
					rspOffset += int(n)
				}
			}
		}

		// 3. Check call invariants
		if strings.HasPrefix(lower, "call ") {
			// Windows x64 ABI requires 32-byte shadow space allocated on stack prior to call
			if isWindows && rspOffset > -32 {
				issues = append(issues, LintIssue{
					Severity:    "WARNING",
					Line:        lineNum,
					Instruction: line,
					Message:     "Windows x64 ABI requires at least 32 bytes (0x20) shadow stack allocation before invoking calls.",
				})
			}
			// Both System V AMD64 and Windows x64 ABI require (RSP % 16 == 0) immediately before CALL
			if rspOffset%16 != 0 {
				issues = append(issues, LintIssue{
					Severity:    "WARNING",
					Line:        lineNum,
					Instruction: line,
					Message:     fmt.Sprintf("Potential x86_64 ABI stack misalignment: RSP offset is %d (not divisible by 16); may trigger GP fault on SSE instructions.", rspOffset),
				})
			}
		}
	}

	return issues
}

