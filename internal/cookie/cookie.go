// Package cookie manages the OpenCode auth cookie.
//
// On first login, the cookie is stored in the OS keychain (macOS Keychain,
// Linux Secret Service via D-Bus, Windows Credential Manager). If no
// keychain is available, it falls back to a plaintext file with a warning.
//
// The non-sensitive workspace ID is always stored in a config file at
// ~/.config/opencode-hawk/config.json (0600).
package cookie

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zalando/go-keyring"
)

const (
	keyringService = "opencode-hawk"
	keyringUser    = "auth-cookie"
	configDir      = "opencode-hawk"
	configFile     = "config.json"
)

// Store holds the workspace ID and storage mode.
type Store struct {
	Cookie      string `json:"-"`
	WorkspaceID string `json:"workspace_id"`
	KeychainOK  bool   `json:"keychain_ok"`
}

// configPath returns the path to the config JSON file under XDG config dir.
func configPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("user config dir: %w", err)
	}
	dir := filepath.Join(base, configDir)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("mkdir: %w", err)
	}
	return filepath.Join(dir, configFile), nil
}

// cookiePath returns the legacy plaintext cookie file path.
func cookiePath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("user config dir: %w", err)
	}
	dir := filepath.Join(base, configDir)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("mkdir: %w", err)
	}
	return filepath.Join(dir, "cookie"), nil
}

// Load reads the stored cookie and workspace ID. Tries keychain first,
// falls back to the legacy plaintext file.
func Load() (*Store, error) {
	s, err := loadConfig()
	if err != nil {
		return nil, err
	}

	// Try keychain.
	if s.KeychainOK {
		c, cerr := keyring.Get(keyringService, keyringUser)
		if cerr == nil && c != "" {
			s.Cookie = c
			return s, nil
		}
		// Keychain failed — fall through to file.
	}

	// Fall back: legacy plaintext file.
	c, ferr := loadLegacyCookie()
	if ferr != nil {
		if errors.Is(ferr, os.ErrNotExist) {
			return nil, fmt.Errorf("not logged in: run 'hawk login' first")
		}
		return nil, ferr
	}
	s.Cookie = c
	s.KeychainOK = false
	return s, nil
}

// Save writes the cookie to the OS keychain (preferred) or a plaintext file
// (fallback). The workspace ID is always written to the config file.
// Returns true if keychain was used.
func Save(cookie, workspaceID string) (bool, error) {
	keychainOK := true
	if err := keyring.Set(keyringService, keyringUser, cookie); err != nil {
		keychainOK = false
		// Fall back to legacy plaintext file.
		if ferr := saveLegacyCookie(cookie); ferr != nil {
			return false, fmt.Errorf("keychain failed and file fallback also failed: %w (file: %v)", err, ferr)
		}
	}

	s := Store{WorkspaceID: workspaceID, KeychainOK: keychainOK}
	if err := saveConfig(s); err != nil {
		// Config file is non-critical for reading, but we've already stored
		// the cookie. Surface the error so the caller can decide.
		return keychainOK, fmt.Errorf("cookie stored but config save failed: %w", err)
	}
	return keychainOK, nil
}

// Delete removes the stored cookie from both keychain and legacy file, and
// deletes the config file.
func Delete() error {
	// Best-effort: try both backends.
	_ = keyring.Delete(keyringService, keyringUser)

	p, err := cookiePath()
	if err == nil {
		_ = os.Remove(p)
	}
	cp, err := configPath()
	if err == nil {
		_ = os.Remove(cp)
	}
	return nil
}

// KeychainWarning returns a warning message when keychain is unavailable.
func KeychainWarning() string {
	var lines []string
	if IsHeadlessLinux() {
		lines = append(lines, "No keychain service detected. On headless Linux, install gnome-keyring")
		lines = append(lines, "or set HAWK_COOKIE to pass the cookie via environment variable.")
	} else {
		lines = append(lines, "System keychain is unavailable. The auth cookie will be stored")
		lines = append(lines, "in plaintext at ~/.config/opencode-hawk/cookie (0600 permissions).")
		lines = append(lines, "Install a keychain service (gnome-keyring, kwallet, etc.) to")
		lines = append(lines, "store it securely.")
	}
	return strings.Join(lines, "\n")
}

// IsHeadlessLinux returns true if we're on Linux without a D-Bus session
// (common in CI, containers, headless servers).
func IsHeadlessLinux() bool {
	// go-keyring uses D-Bus on Linux. If DBUS_SESSION_BUS_ADDRESS is not
	// set, there's no keychain daemon available.
	return os.Getenv("DBUS_SESSION_BUS_ADDRESS") == "" && os.Getenv("XDG_SESSION_TYPE") == ""
}

// ── internal helpers ────────────────────────────────────────────────────

func loadConfig() (*Store, error) {
	cp, err := configPath()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(cp)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("not logged in: run 'hawk login' first")
		}
		return nil, fmt.Errorf("read config: %w", err)
	}
	var s Store
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("config file is corrupted: %w — delete %s and run 'hawk login' again", err, cp)
	}
	if s.WorkspaceID == "" {
		return nil, fmt.Errorf("workspace ID missing: run 'hawk login'")
	}
	return &s, nil
}

func saveConfig(s Store) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	cp, err := configPath()
	if err != nil {
		return err
	}
	if err := os.WriteFile(cp, b, 0600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

func loadLegacyCookie() (string, error) {
	p, err := cookiePath()
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	// Legacy file is raw cookie, not JSON.
	return strings.TrimSpace(string(b)), nil
}

func saveLegacyCookie(cookie string) error {
	p, err := cookiePath()
	if err != nil {
		return err
	}
	return os.WriteFile(p, []byte(cookie), 0600)
}
