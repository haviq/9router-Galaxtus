package dashboard

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"9router/proxy/internal/models"
)

func testUsageConn(provider, authType string) *models.ProviderConnection {
	return &models.ProviderConnection{ID: "t", Provider: provider, AuthType: authType}
}

func TestUsageResetTime(t *testing.T) {
	if got := usageResetTime(nil); got != "" {
		t.Errorf("nil: got %q", got)
	}
	// Unix seconds → RFC3339.
	if got := usageResetTime(float64(1700000000)); !strings.HasPrefix(got, "2023-11-14") {
		t.Errorf("seconds: got %q", got)
	}
	// Unix millis.
	if got := usageResetTime(float64(1700000000000)); !strings.HasPrefix(got, "2023-11-14") {
		t.Errorf("millis: got %q", got)
	}
	// Numeric string.
	if got := usageResetTime("1700000000"); !strings.HasPrefix(got, "2023-11-14") {
		t.Errorf("numeric string: got %q", got)
	}
	// ISO passthrough.
	if got := usageResetTime("2026-01-02T03:04:05Z"); got != "2026-01-02T03:04:05Z" {
		t.Errorf("iso: got %q", got)
	}
	if got := usageResetTime("not-a-date"); got != "" {
		t.Errorf("garbage: got %q", got)
	}
}

func TestMaskConnectionName(t *testing.T) {
	short := "Work account"
	if got := maskConnectionName(short); got != short {
		t.Errorf("short names untouched, got %q", got)
	}
	long := "abcdefghijklmnopqrstuvwxyz0123456789ABCD"
	if got := maskConnectionName(long); got != "abcdefgh***" {
		t.Errorf("token-like masked, got %q", got)
	}
	// Long but not token-like stays.
	human := "This is a fairly long connection name"
	if got := maskConnectionName(human); got != human {
		t.Errorf("human name untouched, got %q", got)
	}
}

func TestParseGroqDurationMs(t *testing.T) {
	ms, ok := parseGroqDurationMs("2m59.56s")
	if !ok || ms < 179000 || ms > 180000 {
		t.Errorf("2m59.56s: got %d,%v", ms, ok)
	}
	if _, ok := parseGroqDurationMs(""); ok {
		t.Errorf("empty must not parse")
	}
	if _, ok := parseGroqDurationMs("tomorrow"); ok {
		t.Errorf("garbage must not parse")
	}
}

func TestGrokMakeQuota(t *testing.T) {
	q := grokMakeQuota(30, 100, "")
	if q["used"] != 30.0 || q["total"] != 100.0 || q["remainingPercentage"] != 70.0 {
		t.Errorf("unexpected quota: %v", q)
	}
	q = grokMakeQuota(5, 0, "")
	if q["total"] != 0 || q["unlimited"] != true {
		t.Errorf("zero total must be unlimited row: %v", q)
	}
}

func TestUsageQuotaShape(t *testing.T) {
	q := usageQuota(25, 100, "2026-01-01T00:00:00Z")
	if q["remainingPercentage"] != 75.0 || q["resetAt"] != "2026-01-01T00:00:00Z" || q["unlimited"] != false {
		t.Errorf("unexpected: %v", q)
	}
}

func TestCodebuddyIsRefill(t *testing.T) {
	// DeductionEndTime is unix ms; gap >2d vs cycle end = refill.
	refill := map[string]any{
		"CycleEndTime":     "2026-09-30 23:59:59",
		"DeductionEndTime": float64(2049542858000),
	}
	if !codebuddyIsRefill(refill) {
		t.Errorf("far-expiry pack must be refill")
	}
	bonus := map[string]any{
		"CycleEndTime":     "2026-09-26 21:27:37",
		"DeductionEndTime": float64(1790429257000),
	}
	if codebuddyIsRefill(bonus) {
		t.Errorf("same-day-expiry pack must be bonus")
	}
}

func TestUsageResetTimeNaiveLocal(t *testing.T) {
	// Naive datetimes parse in server-local zone (mirrors JS new Date(str)).
	got := usageResetTime("2026-09-30 23:59:59")
	want := time.Date(2026, 9, 30, 23, 59, 59, 0, time.Local).UTC().Format(time.RFC3339)
	if got != want {
		t.Errorf("naive local: got %q want %q", got, want)
	}
}

func TestIsUsageEligibleConnection(t *testing.T) {
	cases := []struct {
		provider, authType string
		want               bool
	}{
		{"codex", "oauth", true},
		{"kimi", "apikey", true},
		{"kimi", "oauth", true},
		{"openai", "apikey", true},
		{"deepseek", "apikey", true},
		{"comfyui", "apikey", false},
	}
	for _, c := range cases {
		conn := testUsageConn(c.provider, c.authType)
		if got := isUsageEligibleConnection(conn); got != c.want {
			t.Errorf("%s/%s: got %v want %v", c.provider, c.authType, got, c.want)
		}
	}
}

// Go's transport announces "Go-http-client/1.1", which the WAFs in front of
// chatgpt.com, api.groq.com, api.deepseek.com, openapi.qoder.sh and
// www.codebuddy.ai answer with a bare 403. That surfaced as an opaque
// `Get "…": Forbidden` transport error on the whole quota tracker, so every
// fetcher now announces a real User-Agent unless it sets its own.
func TestUsageDo_SendsNonGoUserAgent(t *testing.T) {
	tests := []struct {
		name    string
		headers map[string]string
		want    string
	}{
		{
			name:    "defaults to the gateway UA",
			headers: map[string]string{"Authorization": "Bearer k"},
			want:    usageUserAgent,
		},
		{
			name:    "a fetcher's own UA is preserved",
			headers: map[string]string{"User-Agent": "grok_cli_rs/1.2.3"},
			want:    "grok_cli_rs/1.2.3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotUA string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotUA = r.Header.Get("User-Agent")
				_, _ = w.Write([]byte(`{}`))
			}))
			defer srv.Close()

			if _, _, _, err := usageGet(t.Context(), srv.URL, tt.headers); err != nil {
				t.Fatalf("usageGet() error = %v", err)
			}
			if gotUA != tt.want {
				t.Errorf("User-Agent = %q, want %q", gotUA, tt.want)
			}
			if strings.Contains(gotUA, "Go-http-client") {
				t.Errorf("User-Agent = %q must not be the Go default", gotUA)
			}
		})
	}
}

func TestFetchRiverUsage_BalanceParsing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/user/balance" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"balance_infos":[{"currency":"USD","total_balance":45.25}]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	res := fetchRiverUsage(t.Context(), "sk-river-test-key", srv.URL+"/v1")
	if res.plan != "River" {
		t.Fatalf("expected plan River, got %q", res.plan)
	}
	q, ok := res.quotas["Balance (USD)"].(map[string]any)
	if !ok {
		t.Fatalf("expected Balance (USD) quota, got %+v", res.quotas)
	}
	if q["isCreditBalance"] != true {
		t.Errorf("expected isCreditBalance true, got %v", q["isCreditBalance"])
	}
	if total, _ := q["total"].(float64); total != 45.25 {
		t.Errorf("expected total 45.25, got %v", total)
	}
}

func TestFetchOpenRouterUsage_CreditsParsing(t *testing.T) {
	// Without key, expect error message
	res := fetchOpenRouterUsage(t.Context(), "")
	if res.message == "" {
		t.Errorf("expected error message for empty key")
	}
}

func TestExtractUniversalBalance(t *testing.T) {
	// 1. balance_infos style
	d1 := map[string]any{
		"balance_infos": []any{
			map[string]any{"currency": "USD", "total_balance": 18.75},
		},
	}
	bal, _, cur, ok := extractUniversalBalance(d1)
	if !ok || bal != 18.75 || cur != "USD" {
		t.Errorf("d1 failed: got %v, %v, %v", bal, cur, ok)
	}

	// 2. nested data style
	d2 := map[string]any{
		"data": map[string]any{
			"total_credits": 50.0,
			"total_usage":   10.0,
		},
	}
	bal, used, _, ok := extractUniversalBalance(d2)
	if !ok || bal != 50.0 || used != 10.0 {
		t.Errorf("d2 failed: got %v, %v, %v", bal, used, ok)
	}

	// 3. direct credit_balance style
	d3 := map[string]any{
		"credit_balance": 33.5,
	}
	bal, _, _, ok = extractUniversalBalance(d3)
	if !ok || bal != 33.5 {
		t.Errorf("d3 failed: got %v, %v", bal, ok)
	}
}

func TestFetchOpenAIUsage_CreditGrants(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/dashboard/billing/credit_grants" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"object": "credit_summary",
				"total_granted": 100.0,
				"total_used": 20.0,
				"total_available": 80.0
			}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	res := fetchOpenAIUsage(t.Context(), "sk-test-openai", srv.URL)
	if !strings.Contains(res.plan, "OpenAI") {
		t.Errorf("expected OpenAI plan, got %q", res.plan)
	}
	q, ok := res.quotas["Balance (USD)"].(map[string]any)
	if !ok {
		t.Fatalf("expected Balance (USD) quota, got %+v", res.quotas)
	}
	if q["isCreditBalance"] != true {
		t.Errorf("expected isCreditBalance true")
	}
	if total, _ := q["total"].(float64); total != 80.0 {
		t.Errorf("expected total 80.0, got %v", total)
	}
}

func TestProbeGenericApiKeyBalance(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/user/balance" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"balance": 42.0, "currency": "USD"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	res, ok := probeGenericApiKeyBalance(t.Context(), "custom-provider", "sk-custom", srv.URL)
	if !ok {
		t.Fatalf("expected probeGenericApiKeyBalance to succeed")
	}
	q, ok := res.quotas["Balance (USD)"].(map[string]any)
	if !ok {
		t.Fatalf("expected Balance (USD) quota, got %+v", res.quotas)
	}
	if q["total"] != 42.0 {
		t.Errorf("expected 42.0, got %v", q["total"])
	}
}
