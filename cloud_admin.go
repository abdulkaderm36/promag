package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// The cloud admin TUI is a small Bubble Tea program for managing a multi-project
// cloud hub: browse projects, create projects, and mint or revoke a per-user
// access token (with a ready-to-share connection string) for any project. It
// replaces the long, repeated `--cloud-* --data-dir … --public-url …` commands.

type adminScreen int

const (
	adminScreenProjects adminScreen = iota
	adminScreenNewProject
	adminScreenManage
	adminScreenAddUsers
	adminScreenInvites
)

type generatedInvite struct {
	ProjectID   string
	ProjectName string
	Label       string
	Invite      string
}

type cloudAdminModel struct {
	registryPath    string
	projectsBaseDir string
	baseURL         string

	screen adminScreen

	projects      []projectRecord
	tokenCounts   map[string]int
	projectCursor int

	selectedProject projectRecord
	tokens          []projectAccessToken
	tokenCursor     int

	nameInput textinput.Model
	usersArea textarea.Model

	batch          []generatedInvite // invites from the most recent add
	sessionInvites []generatedInvite // everything created this session

	status string
	width  int
	height int
}

func runCloudAdmin(registryPath, projectsBaseDir, baseURL string) error {
	model, err := newCloudAdminModel(registryPath, projectsBaseDir, baseURL)
	if err != nil {
		return err
	}
	final, err := tea.NewProgram(model, tea.WithAltScreen()).Run()
	if err != nil {
		return err
	}
	// After the alt screen closes, echo every connection string created this
	// session to stdout so they land in the scrollback and can be copied or
	// piped even if the admin never used the explicit export.
	if fm, ok := final.(cloudAdminModel); ok {
		fm.printSessionInvites()
	}
	return nil
}

func newCloudAdminModel(registryPath, projectsBaseDir, baseURL string) (cloudAdminModel, error) {
	name := textinput.New()
	name.Prompt = ""
	name.CharLimit = 128
	name.Placeholder = "New project name"

	users := textarea.New()
	users.Placeholder = "One username per line, or comma-separated. Paste a whole list here."
	users.CharLimit = 16384
	users.SetWidth(64)
	users.SetHeight(8)

	m := cloudAdminModel{
		registryPath:    registryPath,
		projectsBaseDir: projectsBaseDir,
		baseURL:         baseURL,
		screen:          adminScreenProjects,
		nameInput:       name,
		usersArea:       users,
		tokenCounts:     map[string]int{},
		width:           90,
		height:          28,
	}
	if err := m.reloadProjects(); err != nil {
		return cloudAdminModel{}, err
	}
	return m, nil
}

func (m *cloudAdminModel) reloadProjects() error {
	projects, _, err := loadProjectRegistry(m.registryPath)
	if err != nil {
		return err
	}
	counts := make(map[string]int, len(projects))
	for _, project := range projects {
		tokens, err := loadProjectAccessTokens(m.registryPath, project.ID)
		if err != nil {
			return err
		}
		active := 0
		for _, token := range tokens {
			if token.RevokedAt.IsZero() {
				active++
			}
		}
		counts[project.ID] = active
	}
	m.projects = projects
	m.tokenCounts = counts
	if m.projectCursor >= len(projects) {
		m.projectCursor = max(0, len(projects)-1)
	}
	return nil
}

func (m *cloudAdminModel) reloadTokens() error {
	tokens, err := loadProjectAccessTokens(m.registryPath, m.selectedProject.ID)
	if err != nil {
		return err
	}
	m.tokens = tokens
	if m.tokenCursor >= len(tokens) {
		m.tokenCursor = max(0, len(tokens)-1)
	}
	return nil
}

func (m cloudAdminModel) Init() tea.Cmd { return nil }

func (m cloudAdminModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.usersArea.SetWidth(max(20, min(msg.Width-8, 80)))
		return m, nil
	case tea.KeyMsg:
		switch m.screen {
		case adminScreenProjects:
			return m.updateProjects(msg)
		case adminScreenNewProject:
			return m.updateNewProject(msg)
		case adminScreenManage:
			return m.updateManage(msg)
		case adminScreenAddUsers:
			return m.updateAddUsers(msg)
		case adminScreenInvites:
			return m.updateInvites(msg)
		}
	}
	return m, nil
}

func (m cloudAdminModel) updateProjects(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q", "esc":
		return m, tea.Quit
	case "up", "k":
		if m.projectCursor > 0 {
			m.projectCursor--
		}
	case "down", "j":
		if m.projectCursor < len(m.projects)-1 {
			m.projectCursor++
		}
	case "n":
		m.screen = adminScreenNewProject
		m.nameInput.SetValue("")
		m.nameInput.Focus()
		m.status = ""
	case "enter":
		if len(m.projects) == 0 {
			m.status = "No projects yet. Press n to create one."
			return m, nil
		}
		m.selectedProject = m.projects[m.projectCursor]
		m.tokenCursor = 0
		if err := m.reloadTokens(); err != nil {
			m.status = "Error: " + err.Error()
			return m, nil
		}
		m.screen = adminScreenManage
		m.status = ""
	}
	return m, nil
}

func (m cloudAdminModel) updateNewProject(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.screen = adminScreenProjects
		return m, nil
	case "enter":
		name := strings.TrimSpace(m.nameInput.Value())
		if name == "" {
			m.status = "Project name is required."
			return m, nil
		}
		project, err := createCloudProject(m.registryPath, m.projectsBaseDir, m.projects, name)
		if err != nil {
			m.status = "Error: " + err.Error()
			return m, nil
		}
		if err := m.reloadProjects(); err != nil {
			m.status = "Error: " + err.Error()
			return m, nil
		}
		m.projectCursor = m.projectIndex(project.ID)
		m.screen = adminScreenProjects
		m.status = fmt.Sprintf("Created project %q. Press enter to add users.", project.Name)
		return m, nil
	}
	var cmd tea.Cmd
	m.nameInput, cmd = m.nameInput.Update(msg)
	return m, cmd
}

func (m cloudAdminModel) updateManage(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "esc", "b":
		m.screen = adminScreenProjects
		m.status = ""
		return m, nil
	case "up", "k":
		if m.tokenCursor > 0 {
			m.tokenCursor--
		}
	case "down", "j":
		if m.tokenCursor < len(m.tokens)-1 {
			m.tokenCursor++
		}
	case "a":
		m.screen = adminScreenAddUsers
		m.usersArea.SetValue("")
		m.usersArea.Focus()
		m.status = ""
	case "r":
		if m.tokenCursor >= len(m.tokens) {
			m.status = "No token selected."
			return m, nil
		}
		token := m.tokens[m.tokenCursor]
		if !token.RevokedAt.IsZero() {
			m.status = fmt.Sprintf("Token %q is already revoked.", token.Label)
			return m, nil
		}
		if _, err := revokeProjectAccessTokenForProject(m.registryPath, m.selectedProject.ID, token.ID, localActorID); err != nil {
			m.status = "Error: " + err.Error()
			return m, nil
		}
		if err := m.reloadTokens(); err != nil {
			m.status = "Error: " + err.Error()
			return m, nil
		}
		_ = m.reloadProjects()
		m.status = fmt.Sprintf("Revoked token %q.", token.Label)
	case "e":
		m.status = m.exportInvites(m.selectedProject)
	}
	return m, nil
}

func (m cloudAdminModel) updateAddUsers(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.screen = adminScreenManage
		return m, nil
	case "ctrl+s":
		labels := parseUserLabels(m.usersArea.Value())
		if len(labels) == 0 {
			m.status = "Enter at least one username."
			return m, nil
		}
		m.batch = nil
		for _, label := range labels {
			_, tokenRecord, rawToken, err := createProjectAccessToken(m.registryPath, m.selectedProject.ID, label, localActorID)
			if err != nil {
				m.status = "Error creating token for " + label + ": " + err.Error()
				return m, nil
			}
			remoteURL := cloudProjectRemoteURL(m.baseURL, m.selectedProject.ID)
			invite, err := encodeRemoteInvite(remoteURL, rawToken, m.selectedProject.Name)
			if err != nil {
				m.status = "Error: " + err.Error()
				return m, nil
			}
			gi := generatedInvite{
				ProjectID:   m.selectedProject.ID,
				ProjectName: m.selectedProject.Name,
				Label:       fallback(tokenRecord.Label, label),
				Invite:      invite,
			}
			m.batch = append(m.batch, gi)
			m.sessionInvites = append(m.sessionInvites, gi)
		}
		if err := m.reloadTokens(); err != nil {
			m.status = "Error: " + err.Error()
			return m, nil
		}
		_ = m.reloadProjects()
		m.screen = adminScreenInvites
		m.status = fmt.Sprintf("Created %d connection string(s).", len(m.batch))
		return m, nil
	}
	var cmd tea.Cmd
	m.usersArea, cmd = m.usersArea.Update(msg)
	return m, cmd
}

func (m cloudAdminModel) updateInvites(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "esc", "b":
		m.screen = adminScreenManage
		m.status = ""
	case "e":
		m.status = m.exportInvites(m.selectedProject)
	}
	return m, nil
}

func (m cloudAdminModel) projectIndex(projectID string) int {
	for i, project := range m.projects {
		if project.ID == projectID {
			return i
		}
	}
	return 0
}

// exportInvites writes every connection string created this session for the
// given project to a 0600 file in the working directory. Connection strings
// embed tokens, hence the restrictive permissions.
func (m cloudAdminModel) exportInvites(project projectRecord) string {
	var rows []string
	for _, gi := range m.sessionInvites {
		if gi.ProjectID == project.ID {
			rows = append(rows, gi.Label+"\t"+gi.Invite)
		}
	}
	if len(rows) == 0 {
		return "No connection strings created this session for this project. Press a to add users."
	}
	path := filepath.Join(".", "invites-"+sanitizeFileName(project.Name)+".txt")
	content := "# ProMag connection strings for " + project.Name + "\n# username<TAB>connection-string\n" + strings.Join(rows, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return "Export failed: " + err.Error()
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	return fmt.Sprintf("Exported %d connection string(s) to %s", len(rows), abs)
}

func (m cloudAdminModel) printSessionInvites() {
	if len(m.sessionInvites) == 0 {
		return
	}
	fmt.Fprintf(os.Stdout, "\nGenerated %d connection string(s) this session (share one with each user):\n\n", len(m.sessionInvites))
	for _, gi := range m.sessionInvites {
		fmt.Fprintf(os.Stdout, "[%s] %s\n%s\n\n", gi.ProjectName, gi.Label, gi.Invite)
	}
}

// parseUserLabels splits a pasted list into distinct usernames. Commas,
// semicolons, and newlines separate entries; spaces are preserved so multi-word
// names like "Ali Hassan" stay intact.
func parseUserLabels(raw string) []string {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\r' || r == '\t'
	})
	seen := map[string]bool{}
	var out []string
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		key := strings.ToLower(field)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, field)
	}
	return out
}

func sanitizeFileName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "project"
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "project"
	}
	return strings.ToLower(out)
}

func (m cloudAdminModel) View() string {
	switch m.screen {
	case adminScreenNewProject:
		return m.viewNewProject()
	case adminScreenManage:
		return m.viewManage()
	case adminScreenAddUsers:
		return m.viewAddUsers()
	case adminScreenInvites:
		return m.viewInvites()
	default:
		return m.viewProjects()
	}
}

func (m cloudAdminModel) frame(title string, body []string, hints string) string {
	lines := []string{ui.sectionTitle.Render(title), ""}
	lines = append(lines, body...)
	lines = append(lines, "")
	if m.status != "" {
		lines = append(lines, ui.eyebrow.Render(m.status), "")
	}
	lines = append(lines, ui.subtitle.Render(hints))
	return ui.modalFrame.Render(strings.Join(lines, "\n"))
}

func (m cloudAdminModel) viewProjects() string {
	body := []string{}
	if len(m.projects) == 0 {
		body = append(body, ui.subtitle.Render("No projects yet. Press n to create one."))
	}
	for i, project := range m.projects {
		row := fmt.Sprintf("%-22s  %-20s  %d active token(s)", truncate(project.Name, 22), truncate(project.ID, 20), m.tokenCounts[project.ID])
		if i == m.projectCursor {
			body = append(body, ui.rowSelected.Render("› "+row))
		} else {
			body = append(body, ui.rowIdle.Render("  "+row))
		}
	}
	title := fmt.Sprintf("ProMag Cloud Admin · %d project(s)", len(m.projects))
	return m.frame(title, body, "↑/↓ select · enter manage · n new project · q quit")
}

func (m cloudAdminModel) viewNewProject() string {
	body := []string{
		ui.inputLabel.Render("Project name"),
		m.nameInput.View(),
	}
	return m.frame("New Project", body, "enter create · esc cancel")
}

func (m cloudAdminModel) viewManage() string {
	body := []string{}
	if len(m.tokens) == 0 {
		body = append(body, ui.subtitle.Render("No tokens yet. Press a to add users."))
	}
	for i, token := range m.tokens {
		status := "active"
		if !token.RevokedAt.IsZero() {
			status = "revoked"
		}
		row := fmt.Sprintf("%-24s  %-8s  %s", truncate(fallback(token.Label, "unlabeled"), 24), status, token.CreatedAt.Format("2006-01-02"))
		if i == m.tokenCursor {
			body = append(body, ui.rowSelected.Render("› "+row))
		} else {
			body = append(body, ui.rowIdle.Render("  "+row))
		}
	}
	title := fmt.Sprintf("Manage · %s", m.selectedProject.Name)
	return m.frame(title, body, "a add user(s) · r revoke selected · e export invites · b back · q quit")
}

func (m cloudAdminModel) viewAddUsers() string {
	body := []string{
		ui.subtitle.Render(fmt.Sprintf("Adding users to %s. One token (and connection string) is created per name.", m.selectedProject.Name)),
		"",
		ui.inputLabel.Render("Usernames"),
		m.usersArea.View(),
	}
	return m.frame("Add Users", body, "ctrl+s create tokens · esc cancel")
}

func (m cloudAdminModel) viewInvites() string {
	body := []string{
		ui.subtitle.Render("Share one connection string with each user. They paste it into ProMag (press p, then n)."),
		"",
	}
	display := max(30, min(m.width-12, 88))
	for _, gi := range m.batch {
		body = append(body, ui.eyebrow.Render(gi.Label))
		body = append(body, ui.rowIdle.Render(truncate(gi.Invite, display)))
	}
	body = append(body, "")
	body = append(body, ui.subtitle.Render("Full strings are saved by export and printed when you quit."))
	title := fmt.Sprintf("New Connection Strings · %s", m.selectedProject.Name)
	return m.frame(title, body, "e export to file · b back · q quit")
}
