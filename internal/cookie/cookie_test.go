package cookie

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSaveLoadDelete verifies save (keychain attempt + file fallback),
// load, and delete cycle.
func TestSaveLoadDelete(t *testing.T) {
	dir := t.TempDir()
	os.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))
	defer os.Unsetenv("XDG_CONFIG_HOME")

	// Initial load should fail (not logged in).
	_, err := Load()
	if err == nil {
		t.Fatal("expected error loading before save")
	}

	// Save. In CI / headless, keychain won't be available; Save returns false.
	cookie := "auth=test_cookie_value_12345"
	wid := "wrk_test123"
	keychainOK, err := Save(cookie, wid)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if keychainOK {
		t.Log("keychain available — cookie stored in OS keychain")
	} else {
		t.Log("keychain unavailable — cookie stored as plaintext fallback")
	}

	// Load.
	store, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if store.Cookie != cookie {
		t.Fatalf("cookie: got %q, want %q", store.Cookie, cookie)
	}
	if store.WorkspaceID != wid {
		t.Fatalf("workspace: got %q, want %q", store.WorkspaceID, wid)
	}

	// Config file should exist and be 0600.
	cp, err := configPath()
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(cp)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("expected 0600 perms on config, got %o", info.Mode().Perm())
	}

	// Delete.
	if err := Delete(); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := os.Stat(cp); !os.IsNotExist(err) {
		t.Fatal("expected config file to be removed after delete")
	}

	// Load again should fail.
	_, err = Load()
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

// TestDeleteIdempotent verifies Delete doesn't error when files don't exist.
func TestDeleteIdempotent(t *testing.T) {
	dir := t.TempDir()
	os.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))
	defer os.Unsetenv("XDG_CONFIG_HOME")

	if err := Delete(); err != nil {
		t.Fatalf("delete on non-existent files: %v", err)
	}
}

// TestSaveEmptyWorkspace verifies we can save and load with empty workspace
// (the Load method itself validates this and returns error).
func TestSaveEmptyWorkspace(t *testing.T) {
	dir := t.TempDir()
	os.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))
	defer os.Unsetenv("XDG_CONFIG_HOME")

	keychainOK, err := Save("some_cookie", "")
	if err != nil {
		t.Fatal(err)
	}
	if keychainOK {
		t.Log("keychain used; empty workspace stored only in config")
	}
	_, err = Load()
	if err == nil {
		t.Fatal("expected error for empty workspace ID")
	}
	if !strings.Contains(err.Error(), "workspace ID missing") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestConfigDirCreated verifies the config directory is created on save.
func TestConfigDirCreated(t *testing.T) {
	dir := t.TempDir()
	os.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))
	defer os.Unsetenv("XDG_CONFIG_HOME")

	_, err := configPath()
	if err != nil {
		t.Fatal(err)
	}
	expectedDir := filepath.Join(dir, ".config", configDir)
	if _, err := os.Stat(expectedDir); os.IsNotExist(err) {
		t.Fatal("expected config dir to be created")
	}
}

// TestIsHeadlessLinux verifies the heuristic doesn't crash.
func TestIsHeadlessLinux(t *testing.T) {
	// On a CI runner this should return true; on a desktop it may return
	// false. Either is fine — we just want no panic.
	_ = IsHeadlessLinux()
}
