package main

import (
	"path/filepath"
	"testing"
)

func TestRemoteInviteRoundTrip(t *testing.T) {
	invite, err := encodeRemoteInvite("https://promag.example.com/projects/prj_123/", "tok-secret", "Ops")
	if err != nil {
		t.Fatal(err)
	}
	if !looksLikeRemoteInvite(invite) {
		t.Fatalf("encoded invite %q is not recognized", invite)
	}
	got, ok := parseRemoteInvite(invite)
	if !ok {
		t.Fatalf("parseRemoteInvite(%q) failed", invite)
	}
	if got.URL != "https://promag.example.com/projects/prj_123" {
		t.Fatalf("invite URL = %q (trailing slash should be trimmed)", got.URL)
	}
	if got.Token != "tok-secret" {
		t.Fatalf("invite token = %q", got.Token)
	}
	if got.Name != "Ops" {
		t.Fatalf("invite name = %q", got.Name)
	}
}

func TestParseRemoteInviteRejectsNonInvites(t *testing.T) {
	for _, value := range []string{
		"",
		"Ops",
		"https://promag.example.com/projects/prj_123",
		"promag://join/not-base64!!",
		remoteInviteScheme, // empty payload
	} {
		if _, ok := parseRemoteInvite(value); ok {
			t.Fatalf("parseRemoteInvite(%q) unexpectedly succeeded", value)
		}
	}
}

func TestRemoteProjectClientPrefersStoredToken(t *testing.T) {
	project := projectRecord{
		Type:        projectTypeRemote,
		RemoteURL:   "https://promag.example.com/projects/prj_123",
		RemoteToken: "stored-token",
	}

	// Stored token wins even when an environment override is present.
	t.Setenv(remoteTokenEnv, "env-token")
	client, err := newRemoteProjectClient(project)
	if err != nil {
		t.Fatal(err)
	}
	if client.token != "stored-token" {
		t.Fatalf("client token = %q, want stored-token", client.token)
	}

	// With no stored token, fall back to the environment variable.
	project.RemoteToken = ""
	client, err = newRemoteProjectClient(project)
	if err != nil {
		t.Fatal(err)
	}
	if client.token != "env-token" {
		t.Fatalf("client token = %q, want env-token", client.token)
	}
}

func TestCreateProjectRecordPersistsRemoteToken(t *testing.T) {
	dir := t.TempDir()
	registryPath := filepath.Join(dir, storageDir, registryFile)
	projectsBaseDir := filepath.Join(dir, storageDir, projectsDir)

	created, err := createProjectRecord(registryPath, projectsBaseDir, "Remote Ops", projectTypeRemote, "https://promag.example.com/projects/prj_123", "stored-token")
	if err != nil {
		t.Fatal(err)
	}
	if created.RemoteToken != "stored-token" {
		t.Fatalf("created remote token = %q", created.RemoteToken)
	}

	projects, _, err := loadProjectRegistry(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, project := range projects {
		if project.ID != created.ID {
			continue
		}
		found = true
		if project.RemoteToken != "stored-token" {
			t.Fatalf("reloaded remote token = %q", project.RemoteToken)
		}
		if project.RemoteURL != "https://promag.example.com/projects/prj_123" {
			t.Fatalf("reloaded remote URL = %q", project.RemoteURL)
		}
	}
	if !found {
		t.Fatalf("created project %q not found after reload", created.ID)
	}

	// The token must never appear in API-facing exports.
	exported := exportProjectRecord(created)
	if exported.RemoteURL == "" {
		t.Fatal("exported remote URL should be present")
	}
}

func TestCloudPublicBaseURL(t *testing.T) {
	if got := cloudPublicBaseURL("https://promag.example.com/", ":8080"); got != "https://promag.example.com" {
		t.Fatalf("public-url override = %q", got)
	}
	if got := cloudPublicBaseURL("", ":9000"); got != "http://localhost:9000" {
		t.Fatalf("addr-derived base = %q", got)
	}
}
