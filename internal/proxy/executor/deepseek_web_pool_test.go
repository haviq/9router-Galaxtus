package executor

import (
	"testing"
	"time"
)

func TestParseDeepSeekTokens(t *testing.T) {
	// Single plain token
	t1 := ParseDeepSeekTokens("token123")
	if len(t1) != 1 || t1[0] != "token123" {
		t.Fatalf("expected [token123], got %v", t1)
	}

	// Comma separated
	t2 := ParseDeepSeekTokens("tok1, tok2, tok3")
	if len(t2) != 3 || t2[0] != "tok1" || t2[1] != "tok2" || t2[2] != "tok3" {
		t.Fatalf("expected 3 tokens, got %v", t2)
	}

	// Newline separated
	t3 := ParseDeepSeekTokens("tok1\ntok2\ntok3")
	if len(t3) != 3 {
		t.Fatalf("expected 3 tokens, got %v", t3)
	}

	// JSON array
	t4 := ParseDeepSeekTokens(`["tokA", "tokB"]`)
	if len(t4) != 2 || t4[0] != "tokA" || t4[1] != "tokB" {
		t.Fatalf("expected JSON array parse, got %v", t4)
	}
}

func TestDeepSeekTokenPool_Quarantine(t *testing.T) {
	pool := &DeepSeekTokenPool{slots: make(map[string]*deepSeekAccountSlot)}
	tokens := []string{"acc1", "acc2"}

	// Initially picks acc1 or acc2
	picked1 := pool.PickToken(tokens)
	pool.ReleaseToken(picked1)

	// Quarantine acc1
	pool.QuarantineToken("acc1", 10*time.Second)

	// Next pick should strictly be acc2
	for i := 0; i < 5; i++ {
		picked := pool.PickToken(tokens)
		if picked != "acc2" {
			t.Fatalf("expected acc2 while acc1 is quarantined, got %s", picked)
		}
		pool.ReleaseToken(picked)
	}
}

