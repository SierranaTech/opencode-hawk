---
name: opencode-hawk
description: Usage data scraper for OpenCode Go. Fetches the authenticated dashboard page and extracts rolling/weekly/monthly usage percentages. Outputs JSON for status bars, TUI extensions, and other consumers. First-time setup requires a manual cookie paste from the browser.
---

# opencode-hawk

Scrapes your OpenCode Go usage data from the web dashboard at
`/workspace/<id>/go` and prints it as JSON. The data comes from SolidJS
hydration JSON embedded in the HTML page -- no JavaScript execution, no
reverse-engineered API calls.

## Setup

Run `hawk login` and paste the auth cookie from your browser session
(DevTools > Application > Cookies > opencode.ai > auth value).

The cookie is stored at `~/.config/opencode-hawk/cookie` (0600).

## Usage

```sh
hawk              # fetch once, print JSON to stdout
hawk --out <path>  # write to file for another process
hawk logout        # remove stored cookie
```

## JSON output

```json
{
  "rolling":  {"status":"ok","reset_in_sec":12980,"usage_percent":0},
  "weekly":   {"status":"ok","reset_in_sec":93991,"usage_percent":47},
  "monthly":  {"status":"ok","reset_in_sec":1464013,"usage_percent":54},
  "balance_microcents": 1000000000,
  "fetched_at": "2026-07-11T21:00:00Z"
}
```

Exit code is always 0. On failure, the JSON contains an `error` field.

## Contract

- Three usage windows: rolling (5-hour), weekly, monthly.
- `usagePercent` is 0-100, pre-computed by OpenCode's server against the
  documented $12/$30/$60 limits.
- `resetInSec` gives seconds until the window resets.
- `balance_microcents` is the account balance (1,000,000,000 = $1,000).

## Files of interest

- `cmd/hawk/main.go` -- CLI entry point with login/logout/fetch subcommands
- `internal/scrape/scrape.go` -- HTTP fetch and regex extraction
- `internal/cookie/cookie.go` -- cookie storage in XDG config dir
- `internal/types/types.go` -- JSON output types
