package chat

import (
	"strings"
	"testing"

	"9router/proxy/internal/antiflaky"
	"9router/proxy/internal/db"
)

func TestAntiFlakyGuard_Detector(t *testing.T) {
	// 1. Empty body
	flaky, reason := antiflaky.IsFlakyResponse([]byte(`{"choices":[{"message":{"content":""}}]}`))
	if !flaky || reason != antiflaky.ReasonEmptyBody {
		t.Fatalf("expected empty body, got %v (%s)", flaky, reason)
	}

	// 2. Degenerate repetition loop
	flaky, reason = antiflaky.IsFlakyResponse([]byte(`{"choices":[{"message":{"content":"` + strings.Repeat("OK.", 15) + `"}}]}`))
	if !flaky || reason != antiflaky.ReasonDegenerateLoop {
		t.Fatalf("expected degenerate loop, got %v (%s)", flaky, reason)
	}

	// 3. Normal response
	flaky, _ = antiflaky.IsFlakyResponse([]byte(`{"choices":[{"message":{"content":"func main() {}"}}]}`))
	if flaky {
		t.Fatalf("expected clean response, got flaky")
	}
}

func TestBriefMode_SettingsScoping(t *testing.T) {
	s := &db.SettingsData{
		CavemanEnabled:       true,
		CavemanLevel:         "full",
		BriefModeModelLevels: map[string]string{"deepseek-pro": "ultra", "gpt-4o": "off"},
		BriefModeKeyLevels:   map[string]string{"key-terse": "lite"},
	}

	// Model level beats global
	if s.BriefModeModelLevels["deepseek-pro"] != "ultra" {
		t.Errorf("expected ultra for deepseek-pro")
	}
	if s.BriefModeModelLevels["gpt-4o"] != "off" {
		t.Errorf("expected off for gpt-4o")
	}
	if s.BriefModeKeyLevels["key-terse"] != "lite" {
		t.Errorf("expected lite for key-terse")
	}
}
