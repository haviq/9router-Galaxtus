package antiflaky

import (
	"strings"
	"testing"
)

func TestIsFlakyResponse_Empty(t *testing.T) {
	flaky, reason := IsFlakyResponse([]byte(""))
	if !flaky || reason != ReasonEmptyBody {
		t.Fatalf("expected empty body, got %v (%s)", flaky, reason)
	}

	flaky, reason = IsFlakyResponse([]byte(`   `))
	if !flaky || reason != ReasonEmptyBody {
		t.Fatalf("expected empty body for whitespace, got %v (%s)", flaky, reason)
	}

	jsonEmpty := []byte(`{"id":"chat-1","choices":[{"message":{"role":"assistant","content":""}}]}`)
	flaky, reason = IsFlakyResponse(jsonEmpty)
	if !flaky || reason != ReasonEmptyBody {
		t.Fatalf("expected empty body for empty JSON choice, got %v (%s)", flaky, reason)
	}
}

func TestIsFlakyResponse_DegenerateLoop(t *testing.T) {
	// "OK.OK.OK.OK.OK.OK.OK.OK.OK.OK.OK.OK..."
	repeated := strings.Repeat("OK.", 20)
	flaky, reason := IsFlakyResponse([]byte(repeated))
	if !flaky {
		t.Fatalf("expected degenerate loop for repeated OK., got %v (%s)", flaky, reason)
	}

	jsonLoop := []byte(`{"choices":[{"message":{"content":"` + strings.Repeat("OK.", 15) + `"}}]}`)
	flaky, reason = IsFlakyResponse(jsonLoop)
	if !flaky {
		t.Fatalf("expected degenerate loop for JSON content, got %v (%s)", flaky, reason)
	}
}

func TestIsFlakyResponse_NormalContent(t *testing.T) {
	normalJSON := []byte(`{"choices":[{"message":{"role":"assistant","content":"Here is the Go code to reverse a string:\n\nfunc reverse(s string) string {\n\treturn s\n}"}}]}`)
	flaky, _ := IsFlakyResponse(normalJSON)
	if flaky {
		t.Fatalf("expected non-flaky for normal content, got true")
	}
}
