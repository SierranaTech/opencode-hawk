# opencode-hawk

A Go CLI tool that authenticates with your OpenCode account, maintains a
session cookie, and scrapes your usage and limit data from the workspace
dashboard. Designed to feed real usage stats into the pi footer extension
so the local-session-file approach is no longer needed.

---

## Problem

The pi footer extension currently computes usage from local session files.
This undercounts requests made from other agents, machines, or the OpenCode
IDE plugin. OpenCode doesn't expose a usage/billing API key endpoint — the
data is only available from the authenticated web dashboard at
`/workspace/<id>/go`, which requires a browser session cookie.

## Goal

A `hawk` binary that:
1. Authenticates with OpenCode via a one-time manual cookie paste
2. Stores the session cookie locally (`~/.config/opencode-hawk/cookie`)
3. On each invocation, fetches the workspace page and extracts usage data
   from the embedded SolidJS hydration JSON
4. Writes structured JSON to stdout for the consumer (pi extension, system
   status bar, etc.) to parse and display
5. Cross-platform: linux/amd64 + darwin/arm64 at minimum

---

## Auth flow (OpenAuth / OpenCode)

OpenCode uses [OpenAuth](https://openauth.js.org) with GitHub and Google
providers. The login page at `/auth/authorize` shows two buttons:
- `Continue with GitHub` → `/github/authorize`
- `Continue with Google` → `/google/authorize`

On successful OAuth, the server redirects to a **fixed callback URL**
(`/auth/callback?code=...&state=...`) and sets an `auth` HttpOnly cookie.

The `redirect_uri` is hardcoded to `https://opencode.ai/auth/callback` — no
custom redirect is accepted. This rules out the localhost-capture approach.

### First-time setup: `hawk login`

```
# Run once after install
hawk login

Output:
  Open https://opencode.ai/auth in your browser and log in.
  Then paste the auth cookie value below:
  Cookie: ________________________________
```

The tool validates the cookie by fetching the workspace page (checks for
200 vs 302). On success, stores the cookie to
`~/.config/opencode-hawk/cookie` (plaintext, `0600` perms) alongside the
workspace ID.

### Cookie refresh

After each successful fetch, check the HTTP response for a `Set-Cookie:
auth=...` header. If present, update the stored cookie with the new value.
This handles server-side session rotation automatically.

If a fetch returns a 302 redirect to `/auth/authorize` — or the page HTML
contains `<script>window.location="/auth/authorize"</script>` — the cookie
has expired. Print an error and tell the user to re-authenticate with
`hawk login`.

### Workspace ID

The workspace ID (e.g. `wrk_XXXXXXXXXXXXXXXXXXXX`) is stored alongside
the cookie. Override via `--workspace` flag. Default to prompting on first
login if not provided.

---

## Data source: HTML scrape (decided)

The SolidJS SPA at `/workspace/<id>/go` server-renders all data into the
page HTML via `_$HY.r` hydration JSON. The data is embedded as JavaScript
assignment statements inside a `<script>` tag:

```javascript
$R[28]($R[18],$R[33]={
  rollingUsage: {status:"ok", resetInSec:16075, usagePercent:0},
  weeklyUsage:  {status:"ok", resetInSec:97086, usagePercent:47},
  monthlyUsage: {status:"ok", resetInSec:1467108, usagePercent:54}
});
```

There is also a `/_server?id=<hash>&args=<encoded>` SolidJS server-function
API that returns clean JS (336 bytes vs 16KB HTML). However:

| Dimension | HTML scrape | `_server` API |
|-----------|-------------|---------------|
| Response time | 437ms | 694ms |
| Download size | 16KB | 336 bytes |
| IDs required | none | `x-server-id` + `x-server-instance` headers |
| Stability | always works | IDs change with front-end deploys |

HTML scrape wins: faster, more stable, simpler. The 16KB payload every
60-300s is negligible.

---

## Data layout (discovered via MITM in browser)

### Usage data (from `lite.subscription.get`)

Embedded in the page HTML as:

```javascript
rollingUsage:{status:"ok",resetInSec:16075,usagePercent:0}
weeklyUsage:{status:"ok",resetInSec:97086,usagePercent:47}
monthlyUsage:{status:"ok",resetInSec:1467108,usagePercent:54}
```

| Field | Meaning |
|-------|---------|
| `usagePercent` | 0-100, already computed by OpenCode's server |
| `resetInSec` | seconds until the window resets |
| `status` | `"ok"` when healthy, presumably `"error"` otherwise |

### Billing data (from `billing.get`)

```json
{
  "balance": 1000000000,
  "reload": true,
  "reloadAmount": 10,
  "reloadTrigger": 5,
  "monthlyLimit": 16,
  "paymentMethodLast4": "XXXX"
}
```

`balance` is in microcents (1,000,000,000 microcents = $1,000).

### Dollar limits (for fallback computation)

The page doesn't expose dollar limits. From the OpenCode docs:
- 5-hour window: $12
- Weekly: $30
- Monthly: $60

The `usagePercent` values are computed against these limits by the server.

### Windows

The usage windows are the same rolling windows used for local files.
`resetInSec` gives the exact seconds until the window resets.

---

## Scraping strategy

The tool:

1. Fetches `https://opencode.ai/workspace/<id>/go` with the auth cookie
   and browser-like User-Agent header.
2. Checks response:
   - 302 redirect → cookie expired, report error.
   - 200 OK → embedded data should be present.
   - HTML contains `window.location="/auth/authorize"` → cookie expired.
3. Extracts the three usage objects via targeted regex:
   - `rollingUsage:\$R\[\d+\]=\{status:"[^"]*",resetInSec:(\d+),usagePercent:(\d+)\}`
   - Same pattern for `weeklyUsage` and `monthlyUsage`
4. If regex doesn't match, report error (page structure changed).
5. Checks response `Set-Cookie` header — if a new `auth` cookie is returned,
   updates the stored value.

---

## Output contract

The binary always returns JSON to stdout and exits 0. On failure, includes
an `error` field. The consumer always parses valid JSON.

### Success

```json
{
  "rolling":  {"status":"ok","reset_in_sec":12980,"usage_percent":0},
  "weekly":   {"status":"ok","reset_in_sec":93991,"usage_percent":47},
  "monthly":  {"status":"ok","reset_in_sec":1464013,"usage_percent":54},
  "balance_microcents": 1000000000,
  "fetched_at": "2026-07-11T21:00:00Z"
}
```

### Error

```json
{
  "error": "cookie expired: run 'hawk login'",
  "fetched_at": "2026-07-11T21:00:00Z"
}
```

---

## Implementation

All phases are implemented. See the source files directly.

---

## Go dependencies

Minimal — no third-party runtime deps:

| Concern | Choice |
|---------|--------|
| HTTP client | stdlib `net/http` |
| JSON | stdlib `encoding/json` |
| CLI | stdlib `flag` |
| Config dir | stdlib `os.UserConfigDir()` |
| Regex | stdlib `regexp` — three targeted patterns |

### Cross-platform build

```sh
GOOS=linux GOARCH=amd64 go build -o bin/hawk-linux-amd64 ./cmd/hawk
GOOS=darwin GOARCH=arm64 go build -o bin/hawk-darwin-arm64 ./cmd/hawk
```

Single self-contained binary per arch. No runtime deps beyond libc.

---

## Risk areas

- **SolidJS hydration format could change.** If the front-end updates, the
  variable names (`rollingUsage`, etc.) or assignment pattern might change.
  Validate parsed output and log warnings.
- **Cookie stored in OS keychain.** The auth cookie is stored in the system
  keychain (macOS Keychain, Linux Secret Service/gnome-keyring, Windows
  Credential Manager). On headless Linux without a D-Bus session (CI,
  containers), falls back to plaintext at `~/.config/opencode-hawk/cookie`
  with 0600 permissions and prints a warning on login. The `HAWK_COOKIE`
  env var also works as an escape hatch for secrets managers.
- **Dollar limits not in the page.** The `usagePercent` values are computed
  by the server. If OpenCode changes the limit structure, percentages might
  not match the documented $12/$30/$60 caps. Cross-check against balance
  changes if needed.

---

## Design decisions resolved

| Decision | Chosen | Rationale |
|----------|--------|-----------|
| Data source | HTML scrape | Faster (437ms vs 694ms), more stable than `_server` API |
| Auth flow | Manual cookie paste | OpenAuth redirect_uri is fixed to `/auth/callback` |
| Output contract | Raw JSON to stdout, always exit 0 | Consumer parses valid JSON; `error` field on failures |
| Invocation | Single-shot fetch-and-return | Consumer manages timing |
| Extraction | Targeted regex on _HY.r hydration JSON | 3 simple patterns, no JS execution needed |
| Cross-platform | Go static binary (linux/amd64, darwin/arm64) | Single artifact per arch |

---

## Rollback

The pi extension already has the local-session-file fallback. If hawk breaks
or the page structure changes, the footer continues to show data (possibly
undercounted). No data loss risk — the tool is read-only.
