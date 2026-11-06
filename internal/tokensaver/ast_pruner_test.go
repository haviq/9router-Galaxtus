package tokensaver

import (
	"strings"
	"testing"
)

func TestPruneCodeBlock_LicenseHeaderStripped(t *testing.T) {
	cCode := `/*
 * Copyright (c) 2024 Acme Systems Inc.
 * SPDX-License-Identifier: MIT
 * All rights reserved.
 */

#include <stdio.h>
#include <windows.h>

struct MemoryBlock {
    void* base_addr;
    size_t size;
};

int main() {
    printf("hello world\n");
    return 0;
}
`
	pruned := PruneCodeBlock(cCode, "c")
	if strings.Contains(pruned, "Copyright (c)") {
		t.Fatalf("expected license header to be stripped, got:\n%s", pruned)
	}
	if !strings.Contains(pruned, "struct MemoryBlock") {
		t.Fatalf("expected struct to be preserved, got:\n%s", pruned)
	}
	if !strings.Contains(pruned, "#include <stdio.h>") {
		t.Fatalf("expected include to be preserved, got:\n%s", pruned)
	}
}

func TestCompressCodeContext_FencedBlock(t *testing.T) {
	input := "Here is the code file:\n```c\n/* Copyright 2023 GPL License */\nstruct Test { int a; };\n```\nPlease analyze it."
	// Repeat to exceed length threshold
	input = strings.Repeat("Context padding line\n", 20) + input
	compressed, saved := CompressCodeContext(input)
	if strings.Contains(compressed, "Copyright 2023 GPL License") {
		t.Fatalf("expected boilerplate in fenced code to be removed")
	}
	if !strings.Contains(compressed, "struct Test") {
		t.Fatalf("expected struct Test to remain")
	}
	if saved < 0 {
		t.Fatalf("saved tokens cannot be negative: %d", saved)
	}
}

func TestCompressTextDetailed_ASTCodePrune(t *testing.T) {
	cfg := DefaultRTKConfig()
	var sb strings.Builder
	sb.WriteString("```c\n/*\n * Copyright (c) 2024 EvilCorp\n * All rights reserved.\n */\nstruct Payload {\n    int len;\n};\n```\n")
	for i := 0; i < 30; i++ {
		sb.WriteString("// line " + string(rune('a'+i)) + " padding to test context\n")
	}

	res := CompressTextDetailed(sb.String(), cfg)
	if strings.Contains(res.Text, "Copyright (c) 2024 EvilCorp") {
		t.Fatalf("expected license header to be stripped by ast pruner")
	}
	hasPruneTech := false
	for _, tech := range res.TechniquesUsed {
		if tech == "ast-code-prune" {
			hasPruneTech = true
			break
		}
	}
	if !hasPruneTech {
		t.Fatalf("expected 'ast-code-prune' in TechniquesUsed, got %v", res.TechniquesUsed)
	}
}
