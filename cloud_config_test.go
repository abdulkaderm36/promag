package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("", "  ", "x", "y"); got != "x" {
		t.Fatalf("firstNonEmpty = %q, want x", got)
	}
	if got := firstNonEmpty("", "   "); got != "" {
		t.Fatalf("firstNonEmpty of blanks = %q, want empty", got)
	}
}

func TestLoadCloudConfigFileMissing(t *testing.T) {
	values, err := loadCloudConfigFile(t.TempDir())
	if err != nil {
		t.Fatalf("missing config should not error: %v", err)
	}
	if len(values) != 0 {
		t.Fatalf("missing config should be empty, got %v", values)
	}
}

func TestCloudConfigRoundTrip(t *testing.T) {
	dir := t.TempDir()
	in := map[string]string{
		"PROMAG_SERVER_TOKEN": "secret-token",
		"PROMAG_PUBLIC_URL":   "https://promag.example.com",
	}
	if err := writeCloudConfigFile(dir, in); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(dir, cloudEnvFile)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("config perms = %o, want 600", perm)
	}

	out, err := loadCloudConfigFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	if out["PROMAG_SERVER_TOKEN"] != "secret-token" {
		t.Fatalf("token = %q", out["PROMAG_SERVER_TOKEN"])
	}
	if out["PROMAG_PUBLIC_URL"] != "https://promag.example.com" {
		t.Fatalf("public url = %q", out["PROMAG_PUBLIC_URL"])
	}
}

func TestLoadCloudConfigFileParsing(t *testing.T) {
	dir := t.TempDir()
	content := "# comment\n\nPROMAG_SERVER_TOKEN = \"quoted-token\"\nPROMAG_PUBLIC_URL='https://h.example.com/'\nIGNORED line without equals\n"
	if err := os.WriteFile(filepath.Join(dir, cloudEnvFile), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	values, err := loadCloudConfigFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	if values["PROMAG_SERVER_TOKEN"] != "quoted-token" {
		t.Fatalf("token = %q (quotes/space should be stripped)", values["PROMAG_SERVER_TOKEN"])
	}
	if values["PROMAG_PUBLIC_URL"] != "https://h.example.com/" {
		t.Fatalf("public url = %q", values["PROMAG_PUBLIC_URL"])
	}
	if _, ok := values["IGNORED"]; ok {
		t.Fatal("line without '=' should be ignored")
	}
}
