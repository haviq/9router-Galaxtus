package tokenpool

import (
	"testing"
	"time"
)

func TestParseTokens(t *testing.T) {
	raw := `
		sk-key1, sk-key2; "sk-key3"
		# This is a comment
		// Another comment
		sk-key4
		sk-key1
	`
	tokens := ParseTokens(raw)
	if len(tokens) != 4 {
		t.Fatalf("expected 4 deduplicated tokens, got %d: %v", len(tokens), tokens)
	}
	expected := []string{"sk-key1", "sk-key2", "sk-key3", "sk-key4"}
	for i, want := range expected {
		if tokens[i] != want {
			t.Errorf("token[%d] = %q, want %q", i, tokens[i], want)
		}
	}
}

func TestTokenPool_RotationAndQuarantine(t *testing.T) {
	mgr := GetGlobalManager()
	raw := "key-A, key-B, key-C"
	pool := mgr.GetPool("test-conn-1", raw)

	if pool.Total() != 3 {
		t.Fatalf("expected total 3, got %d", pool.Total())
	}

	// 1. Initial round-robin distribution
	tok1, _, _ := pool.GetNext()
	tok2, _, _ := pool.GetNext()
	tok3, _, _ := pool.GetNext()

	if tok1 == tok2 || tok2 == tok3 || tok1 == tok3 {
		t.Errorf("round robin should cycle through distinct tokens: %s, %s, %s", tok1, tok2, tok3)
	}

	// 2. Quarantine key-B
	pool.Quarantine("key-B", 100*time.Millisecond)

	// Next tokens should only cycle between key-A and key-C
	for i := 0; i < 6; i++ {
		tok, _, avail := pool.GetNext()
		if tok == "key-B" {
			t.Errorf("quarantined token key-B should not be returned while active")
		}
		if avail != 2 {
			t.Errorf("available tokens should be 2, got %d", avail)
		}
	}

	// 3. Quarantine all tokens
	pool.Quarantine("key-A", 100*time.Millisecond)
	pool.Quarantine("key-C", 100*time.Millisecond)

	// When all quarantined, it should fall open rather than hanging
	tokExhausted, _, avail := pool.GetNext()
	if tokExhausted == "" || avail != 0 {
		t.Errorf("exhausted pool should fall open with earliest token, got %q (avail: %d)", tokExhausted, avail)
	}

	// 4. Wait for quarantine expiration
	time.Sleep(120 * time.Millisecond)
	if pool.ActiveCount() != 3 {
		t.Errorf("after cooldown, active count should be 3, got %d", pool.ActiveCount())
	}
}

func TestFirstTokenAndMask(t *testing.T) {
	first := FirstToken("sk-123456789, sk-987654321")
	if first != "sk-123456789" {
		t.Errorf("FirstToken got %q, want sk-123456789", first)
	}

	masked := MaskToken("sk-123456789")
	if masked != "sk-1...6789" {
		t.Errorf("MaskToken got %q, want sk-1...6789", masked)
	}
}

