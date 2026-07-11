// Package scrape fetches the OpenCode workspace page and extracts usage data.
package scrape

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/SierranaTech/opencode-hawk/internal/types"
)

// DefaultUserAgent mimics a browser so the server returns the full SSR page.
const DefaultUserAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/150.0.0.0 Safari/537.36"

const defaultBaseURL = "https://opencode.ai"

var (
	reRolling = regexp.MustCompile(`rollingUsage:\$R\[\d+\]=\{status:"([^"]*)",resetInSec:(\d+),usagePercent:(\d+)\}`)
	reWeekly  = regexp.MustCompile(`weeklyUsage:\$R\[\d+\]=\{status:"([^"]*)",resetInSec:(\d+),usagePercent:(\d+)\}`)
	reMonthly = regexp.MustCompile(`monthlyUsage:\$R\[\d+\]=\{status:"([^"]*)",resetInSec:(\d+),usagePercent:(\d+)\}`)
	reBalance = regexp.MustCompile(`balance:(\d+)`)
	reAuthURL = regexp.MustCompile(`window\.location\s*=\s*"/auth/authorize"`)
)

// Result holds the scraped data plus an optional new cookie from the server.
type Result struct {
	Data      types.Output
	NewCookie string // non-empty if the server returned a fresh auth cookie
}

// Fetch retrieves usage data from the OpenCode workspace dashboard.
// baseURL overrides the production URL; pass empty to use the default.
func Fetch(workspaceID, cookie, baseURL string) (*Result, error) {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	url := fmt.Sprintf("%s/workspace/%s/go", baseURL, workspaceID)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", DefaultUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")
	req.Header.Set("Cookie", "oc_locale=en; auth="+cookie)

	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	// Detect cookie expiry by 302 redirect.
	if resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusSeeOther {
		return nil, fmt.Errorf("cookie expired: run 'hawk login'")
	}
	if resp.StatusCode != http.StatusOK {
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return nil, fmt.Errorf("unexpected status %d (failed to read body: %v)", resp.StatusCode, readErr)
		}
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	html := string(body)

	// Check for client-side redirect to login.
	if reAuthURL.MatchString(html) {
		return nil, fmt.Errorf("cookie expired: run 'hawk login'")
	}

	result := &Result{Data: types.NewOutput()}

	// Check all Set-Cookie headers for a fresh auth cookie.
	for _, c := range resp.Header.Values("Set-Cookie") {
		if _, v, ok := strings.Cut(c, "auth="); ok {
			if semi := strings.IndexByte(v, ';'); semi > 0 {
				result.NewCookie = v[:semi]
			} else {
				result.NewCookie = v
			}
			break
		}
	}

	// Extract usage data.
	result.Data.Rolling = extractUsage(reRolling, html)
	result.Data.Weekly = extractUsage(reWeekly, html)
	result.Data.Monthly = extractUsage(reMonthly, html)

	if result.Data.Rolling == nil && result.Data.Weekly == nil && result.Data.Monthly == nil {
		return result, fmt.Errorf("no usage data found in page - page structure may have changed")
	}

	// Extract balance (best-effort, non-fatal if missing).
	if m := reBalance.FindStringSubmatch(html); len(m) == 2 {
		if v, err := strconv.ParseInt(m[1], 10, 64); err == nil {
			result.Data.BalanceMicroCents = &v
		}
	}

	return result, nil
}

func extractUsage(re *regexp.Regexp, html string) *types.UsageData {
	m := re.FindStringSubmatch(html)
	if len(m) != 4 {
		return nil
	}
	resetInSec, err := strconv.ParseInt(m[2], 10, 64)
	if err != nil {
		return nil
	}
	pct, err := strconv.Atoi(m[3])
	if err != nil {
		return nil
	}
	return &types.UsageData{
		Status:       m[1],
		ResetInSec:   resetInSec,
		UsagePercent: pct,
	}
}
