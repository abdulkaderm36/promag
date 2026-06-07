package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestParseUserLabels(t *testing.T) {
	got := parseUserLabels("ali, sara\nomar ; ali\n\n  Ali Hassan  ")
	want := []string{"ali", "sara", "omar", "Ali Hassan"}
	if len(got) != len(want) {
		t.Fatalf("parseUserLabels returned %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("parseUserLabels[%d] = %q, want %q (full: %v)", i, got[i], want[i], got)
		}
	}
}

func TestSanitizeFileName(t *testing.T) {
	cases := map[string]string{
		"Ops":            "ops",
		"Marketing Team": "marketing-team",
		"  ":             "project",
		"a/b\\c:d":       "abcd",
	}
	for in, want := range cases {
		if got := sanitizeFileName(in); got != want {
			t.Fatalf("sanitizeFileName(%q) = %q, want %q", in, got, want)
		}
	}
}

func keyRunes(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestCloudAdminAddUsersFlow(t *testing.T) {
	dir := t.TempDir()
	registryPath := filepath.Join(dir, storageDir, registryFile)
	projectsBaseDir := filepath.Join(dir, storageDir, projectsDir)

	projects, _, err := loadProjectRegistry(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	project, err := createCloudProject(registryPath, projectsBaseDir, projects, "Ops")
	if err != nil {
		t.Fatal(err)
	}

	model, err := newCloudAdminModel(registryPath, projectsBaseDir, "https://promag.example.com")
	if err != nil {
		t.Fatal(err)
	}

	// enter -> manage the first (only) project
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m := next.(cloudAdminModel)
	if m.screen != adminScreenManage {
		t.Fatalf("after enter, screen = %d, want manage", m.screen)
	}
	if m.selectedProject.ID != project.ID {
		t.Fatalf("selected project = %q, want %q", m.selectedProject.ID, project.ID)
	}

	// a -> add users screen
	next, _ = m.Update(keyRunes("a"))
	m = next.(cloudAdminModel)
	if m.screen != adminScreenAddUsers {
		t.Fatalf("after a, screen = %d, want addUsers", m.screen)
	}

	// type a list and submit with ctrl+s
	m.usersArea.SetValue("ali, sara, omar")
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = next.(cloudAdminModel)

	if m.screen != adminScreenInvites {
		t.Fatalf("after ctrl+s, screen = %d, want invites", m.screen)
	}
	if len(m.batch) != 3 {
		t.Fatalf("created %d invites, want 3", len(m.batch))
	}
	for _, gi := range m.batch {
		invite, ok := parseRemoteInvite(gi.Invite)
		if !ok {
			t.Fatalf("invite for %q does not parse: %q", gi.Label, gi.Invite)
		}
		if invite.Token == "" {
			t.Fatalf("invite for %q has empty token", gi.Label)
		}
		if invite.URL != "https://promag.example.com/projects/"+project.ID {
			t.Fatalf("invite URL = %q", invite.URL)
		}
	}

	// three active tokens now exist in the registry
	tokens, err := loadProjectAccessTokens(registryPath, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	active := 0
	for _, token := range tokens {
		if token.RevokedAt.IsZero() {
			active++
		}
	}
	if active != 3 {
		t.Fatalf("active tokens = %d, want 3", active)
	}

	// export writes a 0600 file with all three connection strings
	t.Chdir(dir)
	status := m.exportInvites(m.selectedProject)
	path := filepath.Join(dir, "invites-ops.txt")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("export status %q but file missing: %v", status, err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("export file perms = %o, want 600", perm)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"ali", "sara", "omar"} {
		if !strings.Contains(string(data), name) {
			t.Fatalf("export file missing %q:\n%s", name, data)
		}
	}
}
