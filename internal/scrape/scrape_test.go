package scrape

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractUsage_Success(t *testing.T) {
	html := readFixture(t, "fixture.html")
	rolling := extractUsage(reRolling, html)
	weekly := extractUsage(reWeekly, html)
	monthly := extractUsage(reMonthly, html)

	if rolling == nil {
		t.Fatal("rollingUsage not found")
	}
	if rolling.Status != "ok" {
		t.Errorf("rolling status: got %q, want %q", rolling.Status, "ok")
	}
	if rolling.ResetInSec != 12980 {
		t.Errorf("rolling resetInSec: got %d, want %d", rolling.ResetInSec, 12980)
	}
	if rolling.UsagePercent != 0 {
		t.Errorf("rolling usagePercent: got %d, want %d", rolling.UsagePercent, 0)
	}

	if weekly == nil {
		t.Fatal("weeklyUsage not found")
	}
	if weekly.UsagePercent != 47 {
		t.Errorf("weekly usagePercent: got %d, want %d", weekly.UsagePercent, 47)
	}
	if weekly.ResetInSec != 93991 {
		t.Errorf("weekly resetInSec: got %d, want %d", weekly.ResetInSec, 93991)
	}

	if monthly == nil {
		t.Fatal("monthlyUsage not found")
	}
	if monthly.UsagePercent != 54 {
		t.Errorf("monthly usagePercent: got %d, want %d", monthly.UsagePercent, 54)
	}
	if monthly.ResetInSec != 1464013 {
		t.Errorf("monthly resetInSec: got %d, want %d", monthly.ResetInSec, 1464013)
	}
}

func TestExtractBalance(t *testing.T) {
	html := readFixture(t, "fixture.html")
	m := reBalance.FindStringSubmatch(html)
	if len(m) != 2 {
		t.Fatal("balance not found")
	}
	if m[1] != "1000000000" {
		t.Errorf("balance: got %s, want %s", m[1], "1000000000")
	}
}

func TestExtractUsage_Expired(t *testing.T) {
	html := readFixture(t, "fixture-expired.html")

	// Usage data should not be found.
	if v := extractUsage(reRolling, html); v != nil {
		t.Error("expected nil rollingUsage for expired cookie")
	}
	if v := extractUsage(reWeekly, html); v != nil {
		t.Error("expected nil weeklyUsage for expired cookie")
	}
	if v := extractUsage(reMonthly, html); v != nil {
		t.Error("expected nil monthlyUsage for expired cookie")
	}

	// Auth redirect should be detected.
	if !reAuthURL.MatchString(html) {
		t.Error("expected auth redirect pattern to match")
	}
}

func TestExtractUsage_EmptyHTML(t *testing.T) {
	if v := extractUsage(reRolling, ""); v != nil {
		t.Error("expected nil for empty HTML")
	}
}

func TestFetch_ServerError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	_, err := Fetch("test-workspace", "test-cookie", ts.URL)
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("expected 500 in error, got: %v", err)
	}
}

func TestFetch_RedirectToAuth(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/auth/authorize")
		w.WriteHeader(http.StatusFound)
	}))
	defer ts.Close()

	_, err := Fetch("test-workspace", "test-cookie", ts.URL)
	if err == nil {
		t.Fatal("expected error for redirect")
	}
	if !strings.Contains(err.Error(), "cookie expired") {
		t.Errorf("expected 'cookie expired' in error, got: %v", err)
	}
}

func TestFetch_AuthRedirectInBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<script>window.location="/auth/authorize"</script>`))
	}))
	defer ts.Close()

	_, err := Fetch("test-workspace", "test-cookie", ts.URL)
	if err == nil {
		t.Fatal("expected error for auth redirect in body")
	}
	if !strings.Contains(err.Error(), "cookie expired") {
		t.Errorf("expected 'cookie expired' in error, got: %v", err)
	}
}

func TestFetch_Success(t *testing.T) {
	fixture := readFixture(t, "fixture.html")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := r.Header.Get("Cookie")
		if c == "" {
			t.Error("expected Cookie header")
		}
		if !strings.Contains(c, "auth=test-cookie") {
			t.Errorf("expected auth cookie in request, got: %s", c)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fixture))
	}))
	defer ts.Close()

	result, err := Fetch("test-workspace", "test-cookie", ts.URL)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Data.Rolling == nil {
		t.Fatal("expected rolling data")
	}
	if result.Data.Rolling.UsagePercent != 0 {
		t.Errorf("rolling pct: got %d, want 0", result.Data.Rolling.UsagePercent)
	}
	if result.Data.Weekly.UsagePercent != 47 {
		t.Errorf("weekly pct: got %d, want 47", result.Data.Weekly.UsagePercent)
	}
	if result.Data.Monthly.UsagePercent != 54 {
		t.Errorf("monthly pct: got %d, want 54", result.Data.Monthly.UsagePercent)
	}
	if result.Data.BalanceMicroCents == nil || *result.Data.BalanceMicroCents != 1000000000 {
		t.Errorf("balance: got %v, want 1000000000", result.Data.BalanceMicroCents)
	}
}

func TestFetch_NoData(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html><body>no data</body></html>"))
	}))
	defer ts.Close()

	_, err := Fetch("test-workspace", "test-cookie", ts.URL)
	if err == nil {
		t.Fatal("expected error for page with no usage data")
	}
}

func TestFetch_NewCookie(t *testing.T) {
	fixture := readFixture(t, "fixture.html")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Set-Cookie", "auth=new-cookie-value; Path=/; HttpOnly")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fixture))
	}))
	defer ts.Close()

	result, err := Fetch("test-workspace", "test-cookie", ts.URL)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if result.NewCookie != "new-cookie-value" {
		t.Errorf("new cookie: got %q, want %q", result.NewCookie, "new-cookie-value")
	}
}

func TestFetch_MultipleSetCookieHeaders(t *testing.T) {
	fixture := readFixture(t, "fixture.html")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// First cookie is something else; auth is second.
		w.Header().Add("Set-Cookie", "oc_locale=en; Path=/")
		w.Header().Add("Set-Cookie", "auth=rotated-cookie; Path=/; HttpOnly")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fixture))
	}))
	defer ts.Close()

	result, err := Fetch("test-workspace", "test-cookie", ts.URL)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if result.NewCookie != "rotated-cookie" {
		t.Errorf("new cookie: got %q, want %q", result.NewCookie, "rotated-cookie")
	}
}

func TestFetch_DefaultBaseURL(t *testing.T) {
	// Passing empty should not panic — it will try the real URL and fail,
	// which is fine for a test that validates the parameter is handled.
	result, err := Fetch("test-workspace", "test-cookie", "")
	_ = result
	if err == nil {
		t.Skip("unexpected success against real server")
	}
	// Error should be from the real server, not from the function itself.
	if strings.Contains(err.Error(), "create request") {
		t.Fatal("should not fail at request creation time")
	}
}

func TestUserAgent(t *testing.T) {
	if DefaultUserAgent == "" {
		t.Fatal("expected non-empty User-Agent")
	}
	if !strings.Contains(DefaultUserAgent, "Chrome") {
		t.Errorf("expected Chrome-like User-Agent, got: %s", DefaultUserAgent)
	}
}

// readFixture reads a test fixture from testdata/.
func readFixture(t *testing.T, name string) string {
	t.Helper()
	p := filepath.Join("..", "..", "testdata", name)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}
