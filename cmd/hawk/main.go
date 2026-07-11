// Command hawk fetches OpenCode Go usage data and prints it as JSON.
//
// Subcommands:
//
//	hawk           — fetch once, print JSON to stdout
//	hawk login     — store auth cookie (interactive)
//	hawk logout    — remove stored auth cookie
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/SierranaTech/opencode-hawk/internal/cookie"
	"github.com/SierranaTech/opencode-hawk/internal/scrape"
	"github.com/SierranaTech/opencode-hawk/internal/types"
)

// Build info — set via ldflags at build time.
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Printf("hawk %s (%s) built %s\n", version, commit, date)
		return
	}
	if len(os.Args) < 2 {
		fetchAndExit()
		return
	}

	switch os.Args[1] {
	case "login":
		login()
	case "logout":
		logout()
	default:
		// Allow flags like --workspace even without subcommand.
		if strings.HasPrefix(os.Args[1], "-") {
			fetchAndExit()
			return
		}
		fmt.Fprintf(os.Stderr, "unknown subcommand: %s\n", os.Args[1])
		fmt.Fprintf(os.Stderr, "usage: hawk [login|logout]\n")
		os.Exit(1)
	}
}

func fetchAndExit() {
	fs := flag.NewFlagSet("hawk", flag.ExitOnError)
	workspace := fs.String("workspace", "", "workspace ID (overrides stored value)")
	out := fs.String("out", "", "write JSON to file instead of stdout")
	_ = fs.Parse(os.Args[1:])

	c, wid := resolveCookie()
	if c == "" {
		writeOutput(types.ErrorOutput("not logged in: run 'hawk login' first"), *out)
		return
	}

	if *workspace != "" {
		wid = *workspace
	}

	result, err := scrape.Fetch(wid, c, "")
	if err != nil {
		writeOutput(types.ErrorOutput(err.Error()), *out)
		return
	}

	// If the server returned a fresh cookie, persist it.
	if result.NewCookie != "" {
		_, _ = cookie.Save(result.NewCookie, wid)
	}

	writeOutput(result.Data, *out)
}

// resolveCookie returns the auth cookie and workspace ID, checking the
// HAWK_COOKIE env var first (for CI/headless use), then the stored config.
func resolveCookie() (string, string) {
	if c := os.Getenv("HAWK_COOKIE"); c != "" {
		wid := os.Getenv("HAWK_WORKSPACE")
		return c, wid
	}
	s, err := cookie.Load()
	if err != nil {
		return "", ""
	}
	return s.Cookie, s.WorkspaceID
}

func login() {
	// Check for HAWK_COOKIE in env — if set, skip interactive prompt.
	if c := os.Getenv("HAWK_COOKIE"); c != "" {
		wid := os.Getenv("HAWK_WORKSPACE")
		if wid == "" {
			fmt.Fprintln(os.Stderr, "HAWK_COOKIE is set but HAWK_WORKSPACE is missing")
			os.Exit(1)
		}
		result, err := scrape.Fetch(wid, c, "")
		if err != nil {
			fmt.Fprintf(os.Stderr, "cookie validation failed: %v\n", err)
			os.Exit(1)
		}
		_ = result
		keychainOK, err := cookie.Save(c, wid)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to save cookie: %v\n", err)
			os.Exit(1)
		}
		if !keychainOK {
			fmt.Fprintln(os.Stderr, cookie.KeychainWarning())
		}
		fmt.Println("Logged in via HAWK_COOKIE.")
		return
	}

	fmt.Println("Open https://opencode.ai/auth in your browser and log in.")
	fmt.Println("Then paste the full auth cookie value below.")
	fmt.Print("Cookie: ")

	var auth string
	_, err := fmt.Scanln(&auth)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading cookie: %v\n", err)
		os.Exit(1)
	}
	auth = strings.TrimSpace(auth)
	if auth == "" {
		fmt.Fprintln(os.Stderr, "cookie cannot be empty")
		os.Exit(1)
	}

	fmt.Print("Workspace ID (e.g. wrk_XXXXXXXXXXXXXXXXXXXX): ")
	var wid string
	_, err = fmt.Scanln(&wid)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading workspace ID: %v\n", err)
		os.Exit(1)
	}
	wid = strings.TrimSpace(wid)
	if wid == "" {
		fmt.Fprintln(os.Stderr, "workspace ID cannot be empty")
		os.Exit(1)
	}

	// Validate the cookie by doing a test fetch.
	_, err = scrape.Fetch(wid, auth, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "cookie validation failed: %v\n", err)
		fmt.Fprintln(os.Stderr, "Make sure the cookie value is correct and you're logged in.")
		os.Exit(1)
	}

	keychainOK, err := cookie.Save(auth, wid)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to save cookie: %v\n", err)
		os.Exit(1)
	}
	if keychainOK {
		fmt.Println("Cookie stored in system keychain.")
	} else {
		fmt.Fprintln(os.Stderr, cookie.KeychainWarning())
	}
	fmt.Println("Logged in. Usage data will now be fetched automatically.")
}

func logout() {
	if err := cookie.Delete(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Logged out.")
}

func writeOutput(data any, outPath string) {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error marshalling JSON: %v\n", err)
		os.Exit(1)
	}
	if outPath != "" {
		if err := os.WriteFile(outPath, b, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "error writing output file: %v\n", err)
			os.Exit(1)
		}
		return
	}
	fmt.Println(string(b))
}
