package coprocessor_test

import (
	"testing"

	"9router/proxy/internal/coprocessor"
)

func TestExtractShellcodeBytes_EscapedHex(t *testing.T) {
	input := `unsigned char buf[] = "\x48\x31\xc0\x48\xff\xc0\xc3";`
	bytes := coprocessor.ExtractShellcodeBytes(input)
	if len(bytes) != 7 {
		t.Fatalf("expected 7 bytes, got %d", len(bytes))
	}
	expected := []byte{0x48, 0x31, 0xc0, 0x48, 0xff, 0xc0, 0xc3}
	for i, b := range bytes {
		if b != expected[i] {
			t.Errorf("byte[%d]: expected 0x%02X, got 0x%02X", i, expected[i], b)
		}
	}
}

func TestExtractShellcodeBytes_CArray(t *testing.T) {
	input := `uint8_t payload[] = { 0x90, 0x90, 0xcc, 0x00 };`
	bytes := coprocessor.ExtractShellcodeBytes(input)
	if len(bytes) != 4 {
		t.Fatalf("expected 4 bytes, got %d", len(bytes))
	}
	if bytes[3] != 0x00 {
		t.Errorf("expected 0x00, got 0x%02X", bytes[3])
	}
}

func TestScanBadBytes(t *testing.T) {
	shellcode := []byte{0x48, 0x31, 0xc0, 0x00, 0x0a, 0xff}
	bad := coprocessor.BadBytesStringSafe // \x00, \x0A, \x0D

	result := coprocessor.ScanBadBytes(shellcode, bad)
	if result.IsClean {
		t.Fatal("expected bad bytes detected, but reported clean")
	}
	if len(result.BadHits) != 2 {
		t.Fatalf("expected 2 bad byte hits, got %d", len(result.BadHits))
	}
	if result.BadHits[0].Offset != 3 || result.BadHits[0].Byte != `\x00` {
		t.Errorf("hit 0 mismatch: %+v", result.BadHits[0])
	}
	if result.BadHits[1].Offset != 4 || result.BadHits[1].Byte != `\x0A` {
		t.Errorf("hit 1 mismatch: %+v", result.BadHits[1])
	}
}

func TestLintX64Assembly_MemToMem(t *testing.T) {
	asm := `
section .text
global _start
_start:
    mov [rax], [rbx]
    ret
`
	issues := coprocessor.LintX64Assembly(asm, "linux")
	if len(issues) == 0 {
		t.Fatal("expected memory-to-memory lint error, got 0 issues")
	}
	if issues[0].Severity != "ERROR" {
		t.Errorf("expected ERROR severity, got %s", issues[0].Severity)
	}
}

func TestLintX64Assembly_StackAlignment(t *testing.T) {
	asm := `
    sub rsp, 24
    call some_function
    add rsp, 24
`
	issues := coprocessor.LintX64Assembly(asm, "linux")
	hasAlignmentWarning := false
	for _, issue := range issues {
		if issue.Severity == "WARNING" && issue.Instruction == "call some_function" {
			hasAlignmentWarning = true
			break
		}
	}
	if !hasAlignmentWarning {
		t.Fatalf("expected stack alignment warning for sub rsp, 24 before call, got %+v", issues)
	}
}

