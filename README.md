# opencode-hawk

Scrape your OpenCode Go usage data from the authenticated dashboard so a status bar, TUI widget, or any other consumer can display it without undercounting.

## Why

OpenCode doesn't expose a usage API key endpoint. The only way to get accurate usage data (including requests from other agents, machines, or the IDE plugin) is from the web dashboard at `/workspace/<id>/go` -- which requires a browser session cookie.

`hawk` is the bridge. You paste your auth cookie once, and the binary fetches the page, extracts the embedded usage data, and prints clean JSON to stdout.

## Install

```sh
# Linux amd64
curl -L -o /usr/local/bin/hawk https://github.com/SierranaTech/opencode-hawk/releases/download/v0.1.0/hawk-linux-amd64
chmod +x /usr/local/bin/hawk

# macOS ARM64
curl -L -o /usr/local/bin/hawk https://github.com/SierranaTech/opencode-hawk/releases/download/v0.1.0/hawk-darwin-arm64
chmod +x /usr/local/bin/hawk
```

Or build from source:

```sh
git clone https://github.com/SierranaTech/opencode-hawk
cd opencode-hawk
go build -o /usr/local/bin/hawk ./cmd/hawk
```

## First-time setup

```sh
hawk login
```

The tool will ask you to:

1. Open `https://opencode.ai/auth` in your browser and log in.
2. Copy the `auth` cookie value from DevTools (Application > Storage > Cookies > opencode.ai > auth).
3. Paste it at the prompt along with your workspace ID.

The cookie is saved to your OS keychain (macOS Keychain, Linux
Secret Service, Windows Credential Manager). If no keychain is available,
it falls back to a plaintext file at `~/.config/opencode-hawk/cookie`
(0600 permissions) with a warning.

## Usage

```sh
hawk                      # fetch once, print JSON to stdout
hawk --out /tmp/usage.json  # write to file for another process to watch
hawk --workspace <id>     # override the stored workspace ID
hawk logout               # remove stored cookie
```

## Output

Always valid JSON. On success:

```json
{
  "rolling":  {"status":"ok","reset_in_sec":12980,"usage_percent":0},
  "weekly":   {"status":"ok","reset_in_sec":93991,"usage_percent":47},
  "monthly":  {"status":"ok","reset_in_sec":1464013,"usage_percent":54},
  "balance_microcents": 10000000,
  "fetched_at": "2026-07-11T21:00:00Z"
}
```

On error:

```json
{
  "error": "cookie expired: run 'hawk login'",
  "fetched_at": "2026-07-11T21:00:00Z"
}
```

Exit code is always 0. The consumer parses JSON and decides what to show.

## Data source

The binary fetches the HTML page at `/workspace/<id>/go` with a browser-like User-Agent and extracts the usage data from the SolidJS `_$HY.r` hydration JSON embedded in the page. Three patterns are matched via regex:

- `rollingUsage` (5-hour rolling window)
- `weeklyUsage` (weekly window, Monday UTC)
- `monthlyUsage` (monthly window, 1st of month UTC)

Each contains `usagePercent` (0-100) and `resetInSec` (seconds until the window resets). The billing blob also contains the account balance.

No JavaScript execution is needed. No reverse-engineered API calls that could break on front-end deploys.

## Build

```sh
GOOS=linux GOARCH=amd64 go build -o bin/hawk-linux-amd64 ./cmd/hawk
GOOS=darwin GOARCH=arm64 go build -o bin/hawk-darwin-arm64 ./cmd/hawk
```

## Test

```sh
go test ./...
```

## Security

The auth cookie is a bearer token for your OpenCode account. It is stored in your OS keychain by default. If no keychain is available, it falls back to a plaintext file at `~/.config/opencode-hawk/cookie` with 0600 permissions. Protect your config directory accordingly.

You can also pass the cookie via the `HAWK_COOKIE` environment variable (with `HAWK_WORKSPACE` for the workspace ID) to avoid any disk storage.

No other data is stored. The tool is read-only -- it only makes GET requests to the OpenCode dashboard.
