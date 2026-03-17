package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
	"github.com/olebedev/when"
	_ "modernc.org/sqlite"
)

const (
	appTitle          = "ProMag"
	dateLayout        = "2006-01-02"
	storageDir        = ".promag"
	registryFile      = "registry.sqlite3"
	projectsDir       = "projects"
	legacyStorageFile = "promag.sqlite3"
	legacyStateFile   = "promag-data.json"
	legacyConfigFile  = "promag-config.json"
	minLeftWidth      = 40
)

var naturalDateParser = when.EN
var ui = newTheme()
var mouseDebugLogPath string
var mouseDebugOverlay bool
var currentLayout layoutState

type viewMode string

const (
	viewTasks   viewMode = "tasks"
	viewMembers viewMode = "members"
	viewDates   viewMode = "dates"
	viewArchive viewMode = "archive"
	viewHelp    viewMode = "help"
)

type overlayMode string

const (
	overlayNone     overlayMode = ""
	overlayActions  overlayMode = "actions"
	overlayMember   overlayMode = "member"
	overlayTask     overlayMode = "task"
	overlayNote     overlayMode = "note"
	overlayFilter   overlayMode = "filter"
	overlaySettings overlayMode = "settings"
	overlayProjects overlayMode = "projects"
)

type projectType string

const (
	projectTypeLocal  projectType = "local"
	projectTypeRemote projectType = "remote"
)

type zone struct {
	X1 int
	Y1 int
	X2 int
	Y2 int
	ID string
}

type member struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Role  string `json:"role"`
	Email string `json:"email"`
}

type task struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	MemberID  string    `json:"member_id"`
	Category  string    `json:"category"`
	Priority  string    `json:"priority"`
	Tags      []string  `json:"tags"`
	Comments  []string  `json:"comments"`
	DueDate   string    `json:"due_date"`
	Status    string    `json:"status"`
	Archived  bool      `json:"archived"`
	CreatedAt time.Time `json:"created_at"`
}

type appState struct {
	Members []member `json:"members"`
	Tasks   []task   `json:"tasks"`
}

type appConfig struct {
	LeftWheelMode string `json:"left_wheel_mode"`
}

type projectRecord struct {
	ID           string
	Name         string
	Type         projectType
	RemoteURL    string
	DBPath       string
	CreatedAt    time.Time
	LastOpenedAt time.Time
}

func defaultConfig() appConfig {
	return appConfig{
		LeftWheelMode: "scroll_list",
	}
}

func (c appConfig) leftWheelMode() string {
	mode := strings.TrimSpace(strings.ToLower(c.LeftWheelMode))
	switch mode {
	case "move_selection", "scroll_list":
		return mode
	default:
		return defaultConfig().LeftWheelMode
	}
}

type row struct {
	Title    string
	Subtitle string
	Meta     string
	Right    string
	ID       string
}

type layoutState struct {
	tabZones     []zone
	rowZones     []zone
	leftZone     zone
	rightZone    zone
	bodyTop      int
	bodyHeight   int
	detailHeight int
	detailWidth  int
}

type model struct {
	dbPath          string
	state           appState
	registryPath    string
	projectsBaseDir string
	projects        []projectRecord
	currentProject  projectRecord

	width   int
	height  int
	bodyTop int

	activeView viewMode
	cursor     map[viewMode]int
	lastStatus string
	statusAt   time.Time

	tabZones     []zone
	rowZones     []zone
	leftZone     zone
	rightZone    zone
	bodyHeight   int
	detailHeight int

	overlay overlayMode

	editingTaskID    string
	editingMemberID  string
	editingProjectID string
	mouseEnabled     bool
	config           appConfig

	memberInputs  []textinput.Model
	taskInputs    []textinput.Model
	filterInputs  []textinput.Model
	noteInputs    []textinput.Model
	noteInput     textarea.Model
	projectInputs []textinput.Model
	formCursor    int

	pendingG      bool
	filter        filterState
	detailScroll  map[viewMode]int
	listOffset    map[viewMode]int
	projectCursor int
	projectCreate bool
	projectLocked bool
}

type filterState struct {
	Text   string
	Member string
	Due    string
}

type theme struct {
	bg              lipgloss.Color
	panel           lipgloss.Color
	panelAlt        lipgloss.Color
	border          lipgloss.Color
	borderStrong    lipgloss.Color
	text            lipgloss.Color
	muted           lipgloss.Color
	subtle          lipgloss.Color
	accent          lipgloss.Color
	accentSoft      lipgloss.Color
	accentContrast  lipgloss.Color
	success         lipgloss.Color
	warn            lipgloss.Color
	danger          lipgloss.Color
	headerFrame     lipgloss.Style
	panelFrame      lipgloss.Style
	panelFrameAlt   lipgloss.Style
	rowSelected     lipgloss.Style
	rowIdle         lipgloss.Style
	modalFrame      lipgloss.Style
	statusFrame     lipgloss.Style
	title           lipgloss.Style
	subtitle        lipgloss.Style
	eyebrow         lipgloss.Style
	sectionTitle    lipgloss.Style
	metricValue     lipgloss.Style
	metricLabel     lipgloss.Style
	tabIdle         lipgloss.Style
	tabActive       lipgloss.Style
	inputLabel      lipgloss.Style
	inputLabelFocus lipgloss.Style
	keycap          lipgloss.Style
}

func newTheme() theme {
	return theme{
		bg:             lipgloss.Color("#08111F"),
		panel:          lipgloss.Color("#0E1A2B"),
		panelAlt:       lipgloss.Color("#111F33"),
		border:         lipgloss.Color("#20334D"),
		borderStrong:   lipgloss.Color("#4E87B8"),
		text:           lipgloss.Color("#ECF3FB"),
		muted:          lipgloss.Color("#93A4B8"),
		subtle:         lipgloss.Color("#6F8098"),
		accent:         lipgloss.Color("#7FDBB6"),
		accentSoft:     lipgloss.Color("#173A39"),
		accentContrast: lipgloss.Color("#08111F"),
		success:        lipgloss.Color("#7FDBB6"),
		warn:           lipgloss.Color("#F6C177"),
		danger:         lipgloss.Color("#F28B82"),
		headerFrame: lipgloss.NewStyle().
			Padding(1, 1, 0, 1),
		panelFrame: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#223753")).
			Padding(1, 2),
		panelFrameAlt: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#28425F")).
			Padding(1, 2),
		rowSelected: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#F8FBFF")).
			Padding(0, 2),
		rowIdle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#D9E6F3")).
			Padding(0, 2),
		modalFrame: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#5C8AB3")).
			Padding(1, 2),
		statusFrame: lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), true, false, false, false).
			BorderForeground(lipgloss.Color("#20334D")).
			Padding(0, 1),
		title: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#F5FAFF")).
			Padding(0, 1),
		subtitle:     lipgloss.NewStyle().Foreground(lipgloss.Color("#93A4B8")),
		eyebrow:      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7FDBB6")),
		sectionTitle: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#D6E4F3")),
		metricValue:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F5FAFF")),
		metricLabel:  lipgloss.NewStyle().Foreground(lipgloss.Color("#6F8098")),
		tabIdle: lipgloss.NewStyle().
			Padding(0, 1).
			Foreground(lipgloss.Color("#AEBED0")),
		tabActive: lipgloss.NewStyle().
			Bold(true).
			Padding(0, 1).
			Background(lipgloss.Color("#14263B")).
			Foreground(lipgloss.Color("#7FDBB6")).
			Underline(true),
		inputLabel:      lipgloss.NewStyle().Foreground(lipgloss.Color("#93A4B8")),
		inputLabelFocus: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7FDBB6")),
		keycap: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#DCE9F6")).
			Background(lipgloss.Color("#14263B")).
			Padding(0, 1),
	}
}

func main() {
	debugFlag := flag.Bool("debug", false, "enable debug logging")
	debugHitboxesFlag := flag.Bool("debug-hitboxes", false, "draw mouse hit zones on screen")
	flag.Parse()

	mouseDebugLogPath = strings.TrimSpace(os.Getenv("PROMAG_DEBUG_MOUSE"))
	if *debugFlag && mouseDebugLogPath == "" {
		mouseDebugLogPath = filepath.Join(os.TempDir(), "promag-mouse.log")
	}
	mouseDebugOverlay = *debugHitboxesFlag || strings.TrimSpace(os.Getenv("PROMAG_DEBUG_HITBOXES")) == "1"

	registryPath := filepath.Join(".", storageDir, registryFile)
	projectsBaseDir := filepath.Join(".", storageDir, projectsDir)
	projects, lastProjectID, err := loadProjectRegistry(registryPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load project registry: %v\n", err)
		os.Exit(1)
	}
	projects, lastProjectID, err = bootstrapDefaultProject(registryPath, projectsBaseDir, projects, lastProjectID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bootstrap project registry: %v\n", err)
		os.Exit(1)
	}

	var (
		activeProject projectRecord
		state         appState
		cfg           appConfig
	)
	if project, ok := chooseActiveProject(projects, lastProjectID); ok {
		activeProject = project
		state, err = loadState(project.DBPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "load state: %v\n", err)
			os.Exit(1)
		}
		cfg, err = loadConfig(project.DBPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "load config: %v\n", err)
			os.Exit(1)
		}
		if err := touchProjectLastOpened(registryPath, project.ID, time.Now()); err != nil {
			fmt.Fprintf(os.Stderr, "update project registry: %v\n", err)
			os.Exit(1)
		}
		projects, _, err = loadProjectRegistry(registryPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "reload project registry: %v\n", err)
			os.Exit(1)
		}
		activeProject, _ = chooseActiveProject(projects, project.ID)
	} else {
		cfg = defaultConfig()
	}

	model := newModel(registryPath, projectsBaseDir, activeProject, projects, state, cfg)
	if width, height, ok := detectTerminalSize(); ok {
		model.width = width
		model.height = height
		model.resizeEditors()
	}

	p := tea.NewProgram(
		model,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "run app: %v\n", err)
		os.Exit(1)
	}

	if (*debugFlag || mouseDebugOverlay) && mouseDebugLogPath != "" {
		fmt.Fprintf(os.Stderr, "debug log: %s\n", mouseDebugLogPath)
	}
}

func newModel(registryPath, projectsBaseDir string, project projectRecord, projects []projectRecord, state appState, cfg appConfig) model {
	memberInputs := make([]textinput.Model, 3)
	memberPlaceholders := []string{"Member name", "Role or specialty", "Email or handle"}
	for i := range memberInputs {
		in := textinput.New()
		in.Placeholder = memberPlaceholders[i]
		in.Prompt = ""
		in.CharLimit = 128
		memberInputs[i] = in
	}
	memberInputs[0].Focus()

	taskInputs := make([]textinput.Model, 7)
	taskPlaceholders := []string{
		"Task title",
		"Members: ali,sara",
		"Category: backend, design, ops",
		"Priority: low, medium, high, urgent",
		"Tags: api,bug,release",
		"Due date: 2026-03-20 or tomorrow",
		"Comments: first note | another note",
	}
	for i := range taskInputs {
		in := textinput.New()
		in.Placeholder = taskPlaceholders[i]
		in.Prompt = ""
		in.CharLimit = 256
		taskInputs[i] = in
	}
	taskInputs[0].Focus()
	taskInputs[1].ShowSuggestions = true

	filterInputs := make([]textinput.Model, 3)
	filterPlaceholders := []string{
		"Text in title, category, tags, comments",
		"Member name",
		"Due date: 2026-03-20 or next friday",
	}
	for i := range filterInputs {
		in := textinput.New()
		in.Placeholder = filterPlaceholders[i]
		in.Prompt = ""
		in.CharLimit = 256
		filterInputs[i] = in
	}
	filterInputs[1].ShowSuggestions = true

	noteInputs := make([]textinput.Model, 2)
	notePlaceholders := []string{
		"Default member for all tasks below",
		"Default due date for all tasks below",
	}
	for i := range noteInputs {
		in := textinput.New()
		in.Placeholder = notePlaceholders[i]
		in.Prompt = ""
		in.CharLimit = 256
		noteInputs[i] = in
	}
	noteInputs[0].ShowSuggestions = true

	noteInput := textarea.New()
	noteInput.Placeholder = notePlaceholder()
	noteInput.SetWidth(80)
	noteInput.SetHeight(14)
	noteInput.Focus()

	projectInputs := make([]textinput.Model, 3)
	projectPlaceholders := []string{
		"Project name",
		"Type: local or remote",
		"Remote URL (required for remote)",
	}
	for i := range projectInputs {
		in := textinput.New()
		in.Placeholder = projectPlaceholders[i]
		in.Prompt = ""
		in.CharLimit = 256
		projectInputs[i] = in
	}

	model := model{
		dbPath:          project.DBPath,
		state:           state,
		registryPath:    registryPath,
		projectsBaseDir: projectsBaseDir,
		projects:        projects,
		currentProject:  project,
		config:          cfg,
		activeView:      viewTasks,
		cursor:          map[viewMode]int{viewTasks: 0, viewMembers: 0, viewDates: 0, viewArchive: 0, viewHelp: 0},
		memberInputs:    memberInputs,
		taskInputs:      taskInputs,
		filterInputs:    filterInputs,
		noteInputs:      noteInputs,
		noteInput:       noteInput,
		projectInputs:   projectInputs,
		mouseEnabled:    true,
		lastStatus:      "Ready. Press ? for the full manual.",
		statusAt:        time.Now(),
		detailScroll:    map[viewMode]int{viewTasks: 0, viewMembers: 0, viewDates: 0, viewArchive: 0, viewHelp: 0},
		listOffset:      map[viewMode]int{viewTasks: 0, viewMembers: 0, viewDates: 0, viewArchive: 0, viewHelp: 0},
	}
	model.refreshMemberSuggestions()
	if len(projects) == 0 {
		model.openProjectCreateForm(true)
		model.lastStatus = "Create your first project to start using ProMag."
	}
	return model
}

func (m model) Init() tea.Cmd {
	return textinput.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resizeEditors()
		return m, nil
	case tea.MouseMsg:
		return m.handleMouse(msg)
	case tea.KeyMsg:
		if m.overlay != overlayNone {
			return m.handleOverlayKey(msg)
		}
		return m.handleNormalKey(msg)
	}

	return m, nil
}

func (m model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	logMouseEvent("mouse event: action=%v button=%v x=%d y=%d overlay=%s mouse_enabled=%t active_view=%s", msg.Action, msg.Button, msg.X, msg.Y, m.overlay, m.mouseEnabled, m.activeView)
	if !m.mouseEnabled {
		logMouseEvent("mouse ignored: mouse disabled")
		return m, nil
	}
	if msg.Button == tea.MouseButtonWheelUp {
		logMouseEvent("mouse wheel: direction=up target=%s action=%v", m.mouseTarget(msg.X, msg.Y), msg.Action)
		return m.scrollByPointer(msg.X, msg.Y, -1), nil
	}
	if msg.Button == tea.MouseButtonWheelDown {
		logMouseEvent("mouse wheel: direction=down target=%s action=%v", m.mouseTarget(msg.X, msg.Y), msg.Action)
		return m.scrollByPointer(msg.X, msg.Y, 1), nil
	}
	if msg.Action != tea.MouseActionPress {
		logMouseEvent("mouse ignored: non-press action without wheel")
		return m, nil
	}
	if msg.Button != tea.MouseButtonLeft || m.overlay != overlayNone {
		logMouseEvent("mouse ignored: button=%v overlay=%s", msg.Button, m.overlay)
		return m, nil
	}

	for _, z := range currentLayout.tabZones {
		if inZone(msg.X, msg.Y, z) {
			logMouseEvent("mouse hit tab: id=%s zone=(%d,%d)-(%d,%d)", z.ID, z.X1, z.Y1, z.X2, z.Y2)
			m.activeView = viewMode(z.ID)
			m.pendingG = false
			m.ensureCursorVisible()
			return m, nil
		}
	}

	for _, z := range currentLayout.rowZones {
		if inZone(msg.X, msg.Y, z) {
			logMouseEvent("mouse hit row: id=%s zone=(%d,%d)-(%d,%d)", z.ID, z.X1, z.Y1, z.X2, z.Y2)
			rows := m.rowsForView()
			for idx, r := range rows {
				if r.ID == z.ID {
					m.cursor[m.activeView] = idx
					m.ensureCursorVisible()
					break
				}
			}
			return m, nil
		}
	}

	logMouseEvent("mouse miss: target=%s tabs=%d rows=%d left_zone=(%d,%d)-(%d,%d) right_zone=(%d,%d)-(%d,%d)", m.mouseTarget(msg.X, msg.Y), len(currentLayout.tabZones), len(currentLayout.rowZones), currentLayout.leftZone.X1, currentLayout.leftZone.Y1, currentLayout.leftZone.X2, currentLayout.leftZone.Y2, currentLayout.rightZone.X1, currentLayout.rightZone.Y1, currentLayout.rightZone.X2, currentLayout.rightZone.Y2)
	return m, nil
}

func (m model) handleOverlayKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.overlay {
	case overlayActions:
		return m.handleActionMenu(msg)
	case overlayMember:
		return m.handleMemberForm(msg)
	case overlayTask:
		return m.handleTaskForm(msg)
	case overlayNote:
		return m.handleNoteForm(msg)
	case overlayFilter:
		return m.handleFilterForm(msg)
	case overlaySettings:
		return m.handleSettingsForm(msg)
	case overlayProjects:
		return m.handleProjectOverlay(msg)
	default:
		return m, nil
	}
}

func (m model) handleNormalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "?":
		m.activeView = viewHelp
		m.setStatus("Manual opened.")
		return m, nil
	case ":":
		m.openActionMenu()
		return m, nil
	case "1":
		m.activeView = viewTasks
		return m, nil
	case "2":
		m.activeView = viewMembers
		return m, nil
	case "3":
		m.activeView = viewDates
		return m, nil
	case "4":
		m.activeView = viewArchive
		return m, nil
	case "5":
		m.activeView = viewHelp
		return m, nil
	case "tab", "l", "right":
		m.nextView()
		m.ensureCursorVisible()
		return m, nil
	case "shift+tab", "h", "left":
		m.prevView()
		m.ensureCursorVisible()
		return m, nil
	case "j", "down":
		return m.moveCursor(1), nil
	case "k", "up":
		return m.moveCursor(-1), nil
	case "ctrl+d", "pagedown":
		return m.scrollDetail(8), nil
	case "ctrl+u", "pageup":
		return m.scrollDetail(-8), nil
	case "d":
		if m.activeView == viewHelp {
			return m.scrollDetail(8), nil
		}
	case "u":
		if m.activeView == viewHelp {
			return m.scrollDetail(-8), nil
		}
	case "g":
		if m.pendingG {
			m.cursor[m.activeView] = 0
			m.pendingG = false
			m.ensureCursorVisible()
			return m, nil
		}
		m.pendingG = true
		return m, nil
	case "G":
		m.cursor[m.activeView] = max(0, len(m.rowsForView())-1)
		m.pendingG = false
		m.ensureCursorVisible()
		return m, nil
	case "]":
		return m.scrollDetail(1), nil
	case "[":
		return m.scrollDetail(-1), nil
	case "J":
		return m.scrollDetail(1), nil
	case "K":
		return m.scrollDetail(-1), nil
	case "a":
		if m.activeView == viewMembers {
			m.openMemberForm()
			return m, nil
		}
		m.openTaskForm(m.taskFormPrefill())
		return m, nil
	case "m":
		m.openMemberForm()
		return m, nil
	case "M":
		m.mouseEnabled = !m.mouseEnabled
		if m.mouseEnabled {
			m.setStatus("Mouse enabled: click and scroll active.")
			return m, tea.EnableMouseCellMotion
		}
		m.setStatus("Mouse disabled: terminal selection/copy available.")
		return m, tea.DisableMouse
	case "e":
		return m.editSelected(), nil
	case "t":
		m.openTaskForm(m.taskFormPrefill())
		return m, nil
	case "n":
		m.openNoteForm()
		return m, nil
	case "p":
		m.openProjectSwitcher()
		return m, nil
	case "s":
		m.openSettings()
		return m, nil
	case "/", "f":
		m.openFilterForm()
		return m, nil
	case "F":
		m.clearFilters()
		return m, nil
	case " ":
		if m.activeView == viewTasks {
			return m.toggleSelectedTask(), nil
		}
		return m, nil
	case "x":
		return m.deleteSelected(), nil
	case "z":
		return m.archiveSelectedTask(), nil
	case "r":
		return m.restoreSelectedTask(), nil
	}

	m.pendingG = false
	return m, nil
}

func (m model) handleMemberForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.closeOverlay("Member entry cancelled.")
		return m, nil
	case "tab", "shift+tab", "enter", "ctrl+j", "ctrl+k", "up", "down":
		s := msg.String()
		if s == "enter" && m.formCursor == len(m.memberInputs)-1 {
			if err := m.submitMemberForm(); err != nil {
				m.setStatus(err.Error())
				return m, nil
			}
			status := "Member saved."
			if m.editingMemberID != "" {
				status = "Member updated."
			}
			m.closeOverlay(status)
			return m, nil
		}
		m.navigateForm(len(m.memberInputs), s)
		return m, nil
	case "ctrl+s":
		if err := m.submitMemberForm(); err != nil {
			m.setStatus(err.Error())
			return m, nil
		}
		status := "Member saved."
		if m.editingMemberID != "" {
			status = "Member updated."
		}
		m.closeOverlay(status)
		return m, nil
	}

	var cmd tea.Cmd
	m.memberInputs[m.formCursor], cmd = m.memberInputs[m.formCursor].Update(msg)
	return m, cmd
}

func (m model) handleTaskForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.closeOverlay("Task entry cancelled.")
		return m, nil
	case "tab", "shift+tab", "ctrl+j", "ctrl+k", "up", "down":
		if msg.String() == "tab" && m.formCursor == 1 && m.taskInputs[1].Focused() && len(m.taskInputs[1].MatchedSuggestions()) > 0 {
			var cmd tea.Cmd
			m.taskInputs[1], cmd = m.taskInputs[1].Update(msg)
			m.refreshMemberSuggestions()
			return m, cmd
		}
		m.navigateForm(len(m.taskInputs), msg.String())
		return m, nil
	case "enter":
		if err := m.submitTaskForm(); err != nil {
			m.setStatus(err.Error())
			return m, nil
		}
		status := "Task saved."
		if m.editingTaskID != "" {
			status = "Task updated."
		}
		m.closeOverlay(status)
		return m, nil
	case "ctrl+s":
		if err := m.submitTaskForm(); err != nil {
			m.setStatus(err.Error())
			return m, nil
		}
		status := "Task saved."
		if m.editingTaskID != "" {
			status = "Task updated."
		}
		m.closeOverlay(status)
		return m, nil
	}

	var cmd tea.Cmd
	m.taskInputs[m.formCursor], cmd = m.taskInputs[m.formCursor].Update(msg)
	if m.formCursor == 1 {
		m.refreshMemberSuggestions()
	}
	return m, cmd
}

func (m model) handleNoteForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.closeOverlay("Quick note capture cancelled.")
		return m, nil
	case "tab", "shift+tab", "ctrl+j", "ctrl+k", "up", "down":
		if msg.String() == "tab" && m.formCursor == 0 && m.noteInputs[0].Focused() && len(m.noteInputs[0].MatchedSuggestions()) > 0 {
			var cmd tea.Cmd
			m.noteInputs[0], cmd = m.noteInputs[0].Update(msg)
			m.refreshMemberSuggestions()
			return m, cmd
		}
		m.navigateForm(len(m.noteInputs)+1, msg.String())
		return m, nil
	case "ctrl+s":
		count, err := m.submitNoteForm()
		if err != nil {
			m.setStatus(err.Error())
			return m, nil
		}
		m.closeOverlay(fmt.Sprintf("%d task(s) created from note capture.", count))
		return m, nil
	}

	if m.formCursor < len(m.noteInputs) {
		var cmd tea.Cmd
		m.noteInputs[m.formCursor], cmd = m.noteInputs[m.formCursor].Update(msg)
		if m.formCursor == 0 {
			m.refreshMemberSuggestions()
		}
		return m, cmd
	}

	var cmd tea.Cmd
	m.noteInput, cmd = m.noteInput.Update(msg)
	return m, cmd
}

func (m model) handleFilterForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.closeOverlay("Filter edit cancelled.")
		return m, nil
	case "tab", "shift+tab", "ctrl+j", "ctrl+k", "up", "down":
		if msg.String() == "tab" && m.formCursor == 1 && m.filterInputs[1].Focused() && len(m.filterInputs[1].MatchedSuggestions()) > 0 {
			var cmd tea.Cmd
			m.filterInputs[1], cmd = m.filterInputs[1].Update(msg)
			m.refreshMemberSuggestions()
			return m, cmd
		}
		m.navigateForm(len(m.filterInputs), msg.String())
		return m, nil
	case "enter":
		if err := m.submitFilterForm(); err != nil {
			m.setStatus(err.Error())
			return m, nil
		}
		m.closeOverlay(m.filterSummary())
		return m, nil
	case "ctrl+s":
		if err := m.submitFilterForm(); err != nil {
			m.setStatus(err.Error())
			return m, nil
		}
		m.closeOverlay(m.filterSummary())
		return m, nil
	case "ctrl+r":
		m.clearFilters()
		m.closeOverlay("Filters cleared.")
		return m, nil
	}

	var cmd tea.Cmd
	m.filterInputs[m.formCursor], cmd = m.filterInputs[m.formCursor].Update(msg)
	if m.formCursor == 1 {
		m.refreshMemberSuggestions()
	}
	return m, cmd
}

func (m model) handleSettingsForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.closeOverlay("Settings cancelled.")
		return m, nil
	case "up", "down", "tab", "shift+tab", "ctrl+j", "ctrl+k":
		if m.config.leftWheelMode() == "scroll_list" {
			m.config.LeftWheelMode = "move_selection"
		} else {
			m.config.LeftWheelMode = "scroll_list"
		}
		return m, nil
	case "enter", "ctrl+s":
		if err := saveConfig(m.dbPath, m.config); err != nil {
			m.setStatus(fmt.Sprintf("save config: %v", err))
			return m, nil
		}
		m.closeOverlay("Settings saved.")
		return m, nil
	}
	return m, nil
}

func (m model) handleProjectOverlay(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.projectCreate {
		switch msg.String() {
		case "esc", "q":
			if m.projectLocked {
				m.setStatus("Create a project to continue.")
				return m, nil
			}
			m.projectCreate = false
			m.formCursor = 0
			m.projectCursor = m.projectIndex(m.currentProject.ID)
			m.setStatus("Project creation cancelled.")
			return m, nil
		case "tab", "shift+tab", "ctrl+j", "ctrl+k", "up", "down":
			m.navigateForm(m.projectFieldCount(), msg.String())
			return m, nil
		case "enter", "ctrl+s":
			project, err := m.submitProjectForm()
			if err != nil {
				m.setStatus(err.Error())
				return m, nil
			}
			if m.editingProjectID == "" {
				if err := m.activateProject(project); err != nil {
					m.setStatus(err.Error())
					return m, nil
				}
				if project.Type == projectTypeRemote {
					m.closeOverlay(fmt.Sprintf("Project %q created. Remote sync is metadata-only for now.", project.Name))
				} else {
					m.closeOverlay(fmt.Sprintf("Project %q created.", project.Name))
				}
				return m, nil
			}
			m.closeOverlay(fmt.Sprintf("Project %q updated.", project.Name))
			return m, nil
		}

		var cmd tea.Cmd
		m.projectInputs[m.formCursor], cmd = m.projectInputs[m.formCursor].Update(msg)
		return m, cmd
	}

	switch msg.String() {
	case "esc", "q":
		if m.projectLocked {
			m.setStatus("Create a project to continue.")
			return m, nil
		}
		m.closeOverlay("Project switcher closed.")
		return m, nil
	case "j", "down", "tab":
		if len(m.projects) == 0 {
			return m, nil
		}
		m.projectCursor = (m.projectCursor + 1) % len(m.projects)
		return m, nil
	case "k", "up", "shift+tab":
		if len(m.projects) == 0 {
			return m, nil
		}
		m.projectCursor--
		if m.projectCursor < 0 {
			m.projectCursor = len(m.projects) - 1
		}
		return m, nil
	case "n":
		m.openProjectCreateForm(false)
		return m, nil
	case "e":
		if len(m.projects) == 0 {
			return m, nil
		}
		m.openProjectEditForm(m.projects[m.projectCursor])
		return m, nil
	case "enter":
		if len(m.projects) == 0 {
			m.openProjectCreateForm(true)
			return m, nil
		}
		project := m.projects[m.projectCursor]
		if err := m.activateProject(project); err != nil {
			m.setStatus(err.Error())
			return m, nil
		}
		m.closeOverlay(fmt.Sprintf("Switched to project %q.", project.Name))
		return m, nil
	}
	return m, nil
}

func (m model) handleActionMenu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.closeOverlay("Action menu closed.")
		return m, nil
	case "q", "ctrl+c":
		return m, tea.Quit
	case "?":
		m.closeOverlay("")
		m.activeView = viewHelp
		m.setStatus("Manual opened.")
		return m, nil
	case "a":
		m.closeOverlay("")
		if m.activeView == viewMembers {
			m.openMemberForm()
		} else {
			m.openTaskForm(m.taskFormPrefill())
		}
		return m, nil
	case "m":
		m.closeOverlay("")
		m.openMemberForm()
		return m, nil
	case "e":
		m.closeOverlay("")
		return m.editSelected(), nil
	case "t":
		m.closeOverlay("")
		m.openTaskForm(m.taskFormPrefill())
		return m, nil
	case "n":
		m.closeOverlay("")
		m.openNoteForm()
		return m, nil
	case "p":
		m.closeOverlay("")
		m.openProjectSwitcher()
		return m, nil
	case "f", "/":
		m.closeOverlay("")
		m.openFilterForm()
		return m, nil
	case "s":
		m.closeOverlay("")
		m.openSettings()
		return m, nil
	case "z":
		m.closeOverlay("")
		return m.archiveSelectedTask(), nil
	case "r":
		m.closeOverlay("")
		return m.restoreSelectedTask(), nil
	case "x":
		m.closeOverlay("")
		return m.deleteSelected(), nil
	}
	return m, nil
}

func (m model) View() string {
	if m.width == 0 || m.height == 0 {
		return ui.subtitle.Render("loading...")
	}

	currentLayout = layoutState{}

	header := m.renderHeader()
	m.bodyTop = lipgloss.Height(header)
	currentLayout.bodyTop = m.bodyTop
	status := m.renderStatus()
	bodyHeight := max(8, m.height-lipgloss.Height(header)-lipgloss.Height(status)-1)
	m.bodyHeight = bodyHeight
	currentLayout.bodyHeight = bodyHeight
	body := m.renderBody(bodyHeight)

	screen := lipgloss.JoinVertical(lipgloss.Left, header, body, status)
	if m.overlay != overlayNone {
		screen = placeOverlay(screen, m.renderOverlay(), m.width, m.height)
	}
	if mouseDebugOverlay {
		screen = renderMouseDebugOverlay(screen, m.width, m.height)
	}
	return screen
}

func (m model) renderHeader() string {
	frameWidth := max(20, m.width)
	contentWidth := max(20, frameWidth-ui.headerFrame.GetHorizontalFrameSize())
	views := []viewMode{viewTasks, viewMembers, viewDates, viewArchive, viewHelp}
	labels := map[viewMode]string{
		viewTasks:   "1 Tasks",
		viewMembers: "2 Team",
		viewDates:   "3 Timeline",
		viewArchive: "4 Archive",
		viewHelp:    "5 Guide",
	}

	logo := m.renderLogo(contentWidth)
	logoHeight := lipgloss.Height(logo)
	tabY := logoHeight + 1
	x := 1
	tabParts := make([]string, 0, len(views))
	for _, v := range views {
		label := labels[v]
		style := ui.tabIdle
		if m.activeView == v {
			style = ui.tabActive
		}
		rendered := style.Render(label)
		w := lipgloss.Width(rendered)
		currentLayout.tabZones = append(currentLayout.tabZones, zone{X1: x, Y1: tabY, X2: x + w - 1, Y2: tabY, ID: string(v)})
		x += w + 1
		tabParts = append(tabParts, rendered)
	}
	tabsLine := strings.Join(tabParts, " ")
	statsLine := lipgloss.JoinHorizontal(
		lipgloss.Top,
		m.renderMetric("Project", fallback(m.currentProject.Name, "none"), ui.accent),
		m.renderMetric("Open", fmt.Sprintf("%d", m.openTaskCount()), ui.warn),
		m.renderMetric("Done", fmt.Sprintf("%d", m.doneTaskCount()), ui.success),
		m.renderMetric("Overdue", fmt.Sprintf("%d", m.overdueTaskCount()), ui.danger),
		m.renderMetric("People", fmt.Sprintf("%d", len(m.state.Members)), ui.borderStrong),
	)
	if lipgloss.Width(tabsLine) > contentWidth {
		tabsLine = ui.subtitle.Render("1-4 switch views")
	}

	lines := []string{logo}
	lines = append(lines, "")
	lines = append(lines, joinHeaderLine(tabsLine, statsLine, contentWidth))
	return ui.headerFrame.Width(frameWidth).Render(strings.Join(lines, "\n"))
}

func (m model) renderBody(bodyHeight int) string {
	availableWidth := max(60, m.width)
	gapWidth := 1
	leftWidth := max(minLeftWidth, min(availableWidth/2-1, 48))
	if leftWidth > availableWidth-32 {
		leftWidth = max(28, availableWidth/2)
	}
	rightWidth := max(30, availableWidth-leftWidth-gapWidth)

	rows := m.rowsForView()
	m.clampCursor(len(rows))
	selected := min(max(m.cursor[m.activeView], 0), max(0, len(rows)-1))

	left := m.renderList(rows, leftWidth, bodyHeight, selected)
	right := m.renderDetail(rightWidth, bodyHeight)
	totalWidth := lipgloss.Width(left) + gapWidth + lipgloss.Width(right)
	if overflow := totalWidth - m.width; overflow > 0 {
		rightWidth = max(30, rightWidth-overflow)
		right = m.renderDetail(rightWidth, bodyHeight)
		totalWidth = lipgloss.Width(left) + gapWidth + lipgloss.Width(right)
	}
	if overflow := totalWidth - m.width; overflow > 0 {
		leftWidth = max(28, leftWidth-overflow)
		left = m.renderList(rows, leftWidth, bodyHeight, selected)
	}
	leftRenderedWidth := lipgloss.Width(left)
	rightRenderedWidth := lipgloss.Width(right)
	m.detailHeight = max(1, bodyHeight-4)
	currentLayout.detailHeight = m.detailHeight
	currentLayout.leftZone = zone{X1: 0, Y1: m.bodyTop, X2: leftRenderedWidth - 1, Y2: m.bodyTop + bodyHeight - 1, ID: "left"}
	currentLayout.rightZone = zone{X1: leftRenderedWidth + gapWidth, Y1: m.bodyTop, X2: leftRenderedWidth + gapWidth + rightRenderedWidth - 1, Y2: m.bodyTop + bodyHeight - 1, ID: "right"}
	return lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right)
}

func (m model) renderList(rows []row, width, height, selected int) string {
	box := ui.panelFrame.Width(width).Height(height)

	title := map[viewMode]string{
		viewTasks:   "Task Queue",
		viewMembers: "Team Directory",
		viewDates:   "Due Timeline",
		viewArchive: "Archived Tasks",
		viewHelp:    "Manual",
	}[m.activeView]

	lines := []string{
		ui.sectionTitle.Render(title),
		ui.subtitle.Render(m.listHint()),
		"",
	}
	frameInsetX := box.GetHorizontalFrameSize() / 2
	frameInsetY := box.GetVerticalFrameSize() / 2
	startY := m.bodyTop + frameInsetY + len(lines)
	y := startY

	if len(rows) == 0 {
		lines = append(lines, ui.subtitle.Render("No records yet. Use a, m, t, or n."))
		return box.Render(strings.Join(lines, "\n"))
	}

	innerHeight := max(1, height-4-len(lines))
	rowHeight := 2
	if m.activeView == viewTasks {
		rowHeight = 3
	}
	rowHeight++
	maxRows := max(1, innerHeight/rowHeight)
	offset := m.listOffset[m.activeView]
	maxOffset := max(0, len(rows)-maxRows)
	if offset < 0 {
		offset = 0
	}
	if offset > maxOffset {
		offset = maxOffset
	}

	for idx := offset; idx < len(rows) && idx < offset+maxRows; idx++ {
		r := rows[idx]
		rowWidth := width - 6
		indicatorWidth := 1
		cardWidth := max(8, rowWidth-indicatorWidth)
		selectedRow := idx == selected
		rowStyle := ui.rowIdle
		if selectedRow {
			rowStyle = ui.rowSelected
		}
		rowContentWidth := cardWidth - rowStyle.GetHorizontalFrameSize()
		rowContentWidth = max(8, rowContentWidth)
		lead := ui.metricLabel.Render(fmt.Sprintf("%02d", idx+1))
		titleWidth := max(8, rowContentWidth-lipgloss.Width(lead)-1)
		headline := lipgloss.JoinHorizontal(lipgloss.Center, lead, " ", truncate(r.Title, titleWidth))
		if r.Right != "" {
			leftWidth := max(10, rowContentWidth-lipgloss.Width(r.Right)-lipgloss.Width(lead)-1)
			headline = joinHeaderLine(
				lipgloss.JoinHorizontal(lipgloss.Center, lead, " ", truncate(r.Title, leftWidth)),
				r.Right,
				rowContentWidth,
			)
		}
		cardLines := []string{
			headline,
			ui.subtitle.Render(truncate(r.Subtitle, rowContentWidth)),
		}
		if r.Meta != "" {
			cardLines = append(cardLines, truncate(r.Meta, rowContentWidth))
		}
		indicator := strings.Repeat(" ", indicatorWidth)
		if selectedRow {
			indicator = lipgloss.NewStyle().Width(indicatorWidth).Foreground(ui.accent).Render("│")
		}
		for _, line := range cardLines {
			lines = append(lines, indicator+rowStyle.Width(cardWidth).Render(line))
		}
		divider := lipgloss.NewStyle().
			Width(cardWidth).
			Padding(0, 2).
			Foreground(ui.borderStrong).
			Render(strings.Repeat("─", max(1, cardWidth-4)))
		lines = append(lines, strings.Repeat(" ", indicatorWidth)+divider)
		rowSpan := 2
		if r.Meta != "" {
			rowSpan = 3
		}
		rowSpan++
		currentLayout.rowZones = append(currentLayout.rowZones, zone{
			X1: frameInsetX,
			Y1: y,
			X2: frameInsetX + indicatorWidth + cardWidth - 1,
			Y2: y + rowSpan - 1,
			ID: r.ID,
		})
		y += rowSpan
	}

	return box.Render(strings.Join(lines, "\n"))
}

func (m model) renderDetail(width, height int) string {
	box := ui.panelFrameAlt.Width(width).Height(height)
	innerWidth := max(1, width-4)
	currentLayout.detailWidth = innerWidth

	content := m.detailContent()
	if m.activeView == viewHelp {
		content = helpManual(innerWidth)
	}
	content = lipgloss.NewStyle().Width(innerWidth).Align(lipgloss.Left).Render(content)
	content = renderViewport(content, innerWidth, max(1, height-4), m.detailScroll[m.activeView])
	return box.Render(content)
}

func (m model) detailContent() string {
	switch m.activeView {
	case viewTasks:
		return m.taskDetail()
	case viewArchive:
		return m.taskDetail()
	case viewMembers:
		return m.memberDetail()
	case viewDates:
		return m.dateDetail()
	case viewHelp:
		return ""
	default:
		return ""
	}
}

func (m model) renderStatus() string {
	frameWidth := max(20, m.width)
	contentWidth := max(20, frameWidth-ui.statusFrame.GetHorizontalFrameSize())
	left := ui.title.Render(truncate(m.lastStatus, max(10, contentWidth/2)))
	center := ui.subtitle.Render(truncate(m.filterSummary(), max(16, contentWidth/3)))
	hints := lipgloss.JoinHorizontal(
		lipgloss.Top,
		ui.keycap.Render(":"),
		" actions ",
		ui.keycap.Render("p"),
		" projects ",
		ui.keycap.Render("s"),
		" settings ",
		ui.keycap.Render("M"),
		" mouse ",
		ui.keycap.Render("q"),
		" quit",
	)
	right := hints
	leftBlock := left
	if strings.TrimSpace(m.filter.Text+m.filter.Member+m.filter.Due) != "" {
		leftBlock = lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", center)
	}
	row := joinHeaderLine(leftBlock, right, contentWidth)
	return ui.statusFrame.Width(frameWidth).Render(row)
}

func (m model) renderOverlay() string {
	bg := ui.modalFrame.Width(min(90, max(54, m.width-10)))

	switch m.overlay {
	case overlayActions:
		lines := []string{
			ui.sectionTitle.Render("Actions"),
			ui.subtitle.Render("Press a key to open the matching modal. esc cancels."),
			"",
			"  t   Add Task",
			"  e   Edit Selected",
			"  m   Add Team Member",
			"  n   Quick Note Capture",
			"  p   Projects",
			"  f   Filters",
			"  s   Settings",
			"  z   Archive Completed Task",
			"  r   Restore Archived Task",
			"  x   Delete Selected",
			"  ?   Manual",
			"  q   Quit",
			"",
			ui.subtitle.Render("a still follows context: member in Team view, task elsewhere."),
		}
		return bg.Render(strings.Join(lines, "\n"))
	case overlayMember:
		var lines []string
		lines = append(lines, ui.sectionTitle.Render("Add Team Member"))
		lines = append(lines, ui.subtitle.Render("tab, arrows, or ctrl+j/ctrl+k to move, ctrl+s to save, esc to cancel"))
		lines = append(lines, "")
		labels := []string{"Name", "Role", "Email"}
		for i, in := range m.memberInputs {
			lines = append(lines, m.formLabel(labels[i], i == m.formCursor))
			lines = append(lines, in.View())
		}
		return bg.Render(strings.Join(lines, "\n"))
	case overlayTask:
		var lines []string
		lines = append(lines, ui.sectionTitle.Render("Add Task"))
		lines = append(lines, ui.subtitle.Render("Members can be comma-separated; this creates one task per member."))
		lines = append(lines, ui.subtitle.Render("tab, arrows, or ctrl+j/ctrl+k to move. enter or ctrl+s saves. esc cancels."))
		lines = append(lines, "")
		labels := []string{"Title", "Members", "Category", "Priority", "Tags", "Due Date", "Comments"}
		for i, in := range m.taskInputs {
			lines = append(lines, m.formLabel(labels[i], i == m.formCursor))
			lines = append(lines, in.View())
		}
		return bg.Render(strings.Join(lines, "\n"))
	case overlayNote:
		lines := []string{
			ui.sectionTitle.Render("Quick Note Capture"),
			ui.subtitle.Render("Scope a batch by member and due date, then write tasks underneath. ctrl+s saves, esc cancels."),
			"",
		}
		labels := []string{"Default Member", "Default Due Date", "Task Notes"}
		for i, label := range labels {
			lines = append(lines, m.formLabel(label, i == m.formCursor))
			if i < len(m.noteInputs) {
				lines = append(lines, m.noteInputs[i].View())
				continue
			}
			lines = append(lines, m.noteInput.View())
		}
		return bg.Render(strings.Join(lines, "\n"))
	case overlayFilter:
		var lines []string
		lines = append(lines, ui.sectionTitle.Render("Filters"))
		lines = append(lines, ui.subtitle.Render("Filter by text, member, or due date. enter or ctrl+s applies. ctrl+r clears. esc cancels."))
		lines = append(lines, "")
		labels := []string{"Text", "Member", "Due Date"}
		for i, in := range m.filterInputs {
			lines = append(lines, m.formLabel(labels[i], i == m.formCursor))
			lines = append(lines, in.View())
		}
		return bg.Render(strings.Join(lines, "\n"))
	case overlaySettings:
		mode := m.config.leftWheelMode()
		scrollList := ui.subtitle.Render("scroll_list")
		moveSelection := ui.subtitle.Render("move_selection")
		if mode == "scroll_list" {
			scrollList = ui.eyebrow.Render("scroll_list")
		}
		if mode == "move_selection" {
			moveSelection = ui.eyebrow.Render("move_selection")
		}
		lines := []string{
			ui.sectionTitle.Render("Settings"),
			ui.subtitle.Render("up/down or tab toggles. enter or ctrl+s saves. esc cancels."),
			"",
			ui.inputLabelFocus.Render("Left wheel behavior"),
			"  " + scrollList,
			"  " + moveSelection,
			"",
			ui.subtitle.Render("scroll_list keeps the selected task pinned while the list viewport moves."),
			ui.subtitle.Render("move_selection makes the wheel move the selected row directly."),
		}
		return bg.Render(strings.Join(lines, "\n"))
	case overlayProjects:
		if m.projectCreate {
			title := "Create Project"
			subtitle := "Name the project, choose local or remote, then save. esc cancels unless this is first launch."
			if m.editingProjectID != "" {
				title = "Edit Project"
				subtitle = "Update the project details, then save. esc returns to the project list."
			}
			lines := []string{
				ui.sectionTitle.Render(title),
				ui.subtitle.Render(subtitle),
				"",
			}
			labels := []string{"Name", "Type", "Remote URL"}
			for i := 0; i < m.projectFieldCount(); i++ {
				lines = append(lines, m.formLabel(labels[i], i == m.formCursor))
				lines = append(lines, m.projectInputs[i].View())
			}
			if normalizeProjectType(m.projectInputs[1].Value()) == projectTypeRemote {
				lines = append(lines, "")
				lines = append(lines, ui.subtitle.Render("Remote projects still use a local cache DB for now."))
			}
			return bg.Render(strings.Join(lines, "\n"))
		}

		lines := []string{
			ui.sectionTitle.Render("Projects"),
			ui.subtitle.Render("j/k or tab move, enter switches, n creates, e edits, esc closes."),
			"",
		}
		if len(m.projects) == 0 {
			lines = append(lines, ui.subtitle.Render("No projects yet. Press n to create one."))
			return bg.Render(strings.Join(lines, "\n"))
		}
		for i, project := range m.projects {
			prefix := "  "
			typeLabel := projectTypeBadge(project.Type)
			label := ui.subtitle.Render(typeLabel + " " + project.Name)
			if i == m.projectCursor {
				prefix = lipgloss.NewStyle().Foreground(ui.accentSoft).Render("› ")
				label = ui.eyebrow.Render(typeLabel + " " + project.Name)
			}
			if project.ID == m.currentProject.ID {
				current := lipgloss.NewStyle().Foreground(ui.success).Bold(true).Render("current")
				label = label + ui.subtitle.Render(" • ") + current
			}
			meta := ""
			if project.RemoteURL != "" {
				meta = truncate(project.RemoteURL, 40)
			}
			lines = append(lines, prefix+label)
			if meta != "" {
				lines = append(lines, "  "+ui.subtitle.Render(meta))
			}
		}
		return bg.Render(strings.Join(lines, "\n"))
	default:
		return ""
	}
}

func (m model) rowsForView() []row {
	switch m.activeView {
	case viewTasks:
		tasks := m.filteredTasks()
		rows := make([]row, 0, len(tasks))
		for _, t := range tasks {
			memberName := m.memberName(t.MemberID)
			rows = append(rows, row{
				ID:       t.ID,
				Title:    t.Title,
				Subtitle: fmt.Sprintf("%s  •  %s", fallback(memberName, "Unassigned"), dueLabel(t.DueDate)),
				Meta:     lipgloss.JoinHorizontal(lipgloss.Top, statusPill(t.Status), ui.subtitle.Render(" • "), priorityPill(t.Priority)),
			})
		}
		return rows
	case viewArchive:
		tasks := m.archivedTasks()
		rows := make([]row, 0, len(tasks))
		for _, t := range tasks {
			memberName := m.memberName(t.MemberID)
			rows = append(rows, row{
				ID:       t.ID,
				Title:    t.Title,
				Subtitle: fmt.Sprintf("%s  •  %s", fallback(memberName, "Unassigned"), dueLabel(t.DueDate)),
				Meta:     lipgloss.JoinHorizontal(lipgloss.Top, statusPill("archived"), ui.subtitle.Render(" • "), priorityPill(t.Priority)),
			})
		}
		return rows
	case viewMembers:
		members := m.filteredMembers()
		sort.Slice(members, func(i, j int) bool {
			return strings.ToLower(members[i].Name) < strings.ToLower(members[j].Name)
		})
		rows := make([]row, 0, len(members))
		for _, member := range members {
			open, done := m.filteredMemberTaskCounts(member.ID)
			rows = append(rows, row{
				ID:       member.ID,
				Title:    fmt.Sprintf("%s  %s", memberBadge(member.Name), member.Name),
				Subtitle: fmt.Sprintf("%s  •  open %d  •  done %d", fallback(member.Role, "no role"), open, done),
			})
		}
		return rows
	case viewDates:
		dates := m.groupedDates()
		rows := make([]row, 0, len(dates))
		for _, g := range dates {
			rows = append(rows, row{
				ID:       g.Date,
				Title:    fmt.Sprintf("%s  (%d task%s)", dueLabel(g.Date), len(g.Tasks), plural(len(g.Tasks))),
				Subtitle: gSummary(g.Tasks, m),
			})
		}
		return rows
	case viewHelp:
		return []row{
			{ID: "manual", Title: "Keyboard, mouse, quick capture, data model", Subtitle: "The full help text is on the right."},
		}
	default:
		return nil
	}
}

func (m *model) clampCursor(count int) {
	if count <= 0 {
		m.cursor[m.activeView] = 0
		return
	}
	if m.cursor[m.activeView] < 0 {
		m.cursor[m.activeView] = 0
	}
	if m.cursor[m.activeView] >= count {
		m.cursor[m.activeView] = count - 1
	}
}

func (m model) moveCursor(delta int) model {
	rows := m.rowsForView()
	if len(rows) == 0 {
		return m
	}
	m.cursor[m.activeView] += delta
	m.clampCursor(len(rows))
	m.ensureCursorVisible()
	return m
}

func (m model) scrollByPointer(x, y, delta int) model {
	if inZone(x, y, currentLayout.rightZone) {
		logMouseEvent("scroll target: right detail pane")
		return m.scrollDetail(delta)
	}
	if m.config.leftWheelMode() == "scroll_list" {
		logMouseEvent("scroll target: left list viewport")
		return m.scrollList(delta)
	}
	logMouseEvent("scroll target: left list pane")
	return m.moveCursor(delta)
}

func (m model) mouseTarget(x, y int) string {
	for _, z := range currentLayout.tabZones {
		if inZone(x, y, z) {
			return "tab:" + z.ID
		}
	}
	if inZone(x, y, currentLayout.leftZone) {
		return "left"
	}
	if inZone(x, y, currentLayout.rightZone) {
		return "right"
	}
	return "none"
}

func (m model) scrollDetail(delta int) model {
	contentHeight := detailContentHeight(m.detailViewportContent())
	visibleHeight := max(1, currentLayout.detailHeight)
	maxScroll := max(0, contentHeight-visibleHeight)
	next := m.detailScroll[m.activeView] + delta
	if next < 0 {
		next = 0
	}
	if next > maxScroll {
		next = maxScroll
	}
	m.detailScroll[m.activeView] = next
	return m
}

func (m model) scrollList(delta int) model {
	rows := m.rowsForView()
	maxOffset := max(0, len(rows)-m.maxVisibleRows())
	next := m.listOffset[m.activeView] + delta
	if next < 0 {
		next = 0
	}
	if next > maxOffset {
		next = maxOffset
	}
	m.listOffset[m.activeView] = next
	return m
}

func (m *model) ensureCursorVisible() {
	maxRows := m.maxVisibleRows()
	if maxRows <= 0 {
		return
	}
	offset := m.listOffset[m.activeView]
	cursor := m.cursor[m.activeView]
	if cursor < offset {
		m.listOffset[m.activeView] = cursor
		return
	}
	if cursor >= offset+maxRows {
		m.listOffset[m.activeView] = cursor - maxRows + 1
	}
}

func (m model) maxVisibleRows() int {
	headerLines := 3
	innerHeight := max(1, m.bodyHeight-4-headerLines)
	rowHeight := 2
	if m.activeView == viewTasks {
		rowHeight = 3
	}
	rowHeight++
	return max(1, innerHeight/rowHeight)
}

func (m model) detailViewportContent() string {
	innerWidth := max(1, currentLayout.detailWidth)
	content := m.detailContent()
	if m.activeView == viewHelp {
		content = helpManual(innerWidth)
	}
	return lipgloss.NewStyle().Width(innerWidth).Align(lipgloss.Left).Render(content)
}

func (m *model) nextView() {
	views := []viewMode{viewTasks, viewMembers, viewDates, viewArchive, viewHelp}
	idx := slices.Index(views, m.activeView)
	m.activeView = views[(idx+1)%len(views)]
}

func (m *model) prevView() {
	views := []viewMode{viewTasks, viewMembers, viewDates, viewArchive, viewHelp}
	idx := slices.Index(views, m.activeView)
	if idx <= 0 {
		m.activeView = views[len(views)-1]
		return
	}
	m.activeView = views[idx-1]
}

func (m *model) setStatus(s string) {
	m.lastStatus = s
	m.statusAt = time.Now()
}

func (m *model) closeOverlay(status string) {
	m.overlay = overlayNone
	m.formCursor = 0
	m.editingTaskID = ""
	m.editingMemberID = ""
	m.editingProjectID = ""
	m.projectCreate = false
	m.projectLocked = false
	for i := range m.memberInputs {
		m.memberInputs[i].Blur()
	}
	for i := range m.taskInputs {
		m.taskInputs[i].Blur()
	}
	for i := range m.filterInputs {
		m.filterInputs[i].Blur()
	}
	for i := range m.noteInputs {
		m.noteInputs[i].Blur()
	}
	m.noteInput.Blur()
	for i := range m.projectInputs {
		m.projectInputs[i].Blur()
	}
	if status != "" {
		m.setStatus(status)
	}
}

func (m *model) openMemberForm() {
	m.overlay = overlayMember
	m.formCursor = 0
	m.editingMemberID = ""
	for i := range m.memberInputs {
		m.memberInputs[i].SetValue("")
		m.memberInputs[i].Blur()
	}
	m.memberInputs[0].Focus()
}

func (m *model) openTaskForm(defaultMembers, defaultDueDate string) {
	m.overlay = overlayTask
	m.formCursor = 0
	m.editingTaskID = ""
	for i := range m.taskInputs {
		m.taskInputs[i].SetValue("")
		m.taskInputs[i].Blur()
	}
	if defaultMembers != "" {
		m.taskInputs[1].SetValue(defaultMembers)
	}
	if defaultDueDate != "" {
		m.taskInputs[5].SetValue(defaultDueDate)
	}
	m.refreshMemberSuggestions()
	m.taskInputs[0].Focus()
}

func (m *model) openNoteForm() {
	m.overlay = overlayNote
	m.formCursor = 0
	defaultMember, defaultDueDate := m.taskFormPrefill()
	for i := range m.noteInputs {
		m.noteInputs[i].SetValue("")
		m.noteInputs[i].Blur()
	}
	m.noteInputs[0].SetValue(defaultMember)
	m.noteInputs[1].SetValue(defaultDueDate)
	m.noteInput.SetValue("")
	m.noteInput.Blur()
	m.refreshMemberSuggestions()
	m.noteInputs[0].Focus()
}

func (m *model) openFilterForm() {
	m.overlay = overlayFilter
	m.formCursor = 0
	values := []string{m.filter.Text, m.filter.Member, m.filter.Due}
	for i := range m.filterInputs {
		m.filterInputs[i].SetValue(values[i])
		m.filterInputs[i].Blur()
	}
	m.refreshMemberSuggestions()
	m.filterInputs[0].Focus()
}

func (m *model) openMemberEditForm(selected *member) {
	if selected == nil {
		return
	}
	m.overlay = overlayMember
	m.formCursor = 0
	m.editingMemberID = selected.ID
	for i := range m.memberInputs {
		m.memberInputs[i].SetValue("")
		m.memberInputs[i].Blur()
	}
	m.memberInputs[0].SetValue(selected.Name)
	m.memberInputs[1].SetValue(selected.Role)
	m.memberInputs[2].SetValue(selected.Email)
	m.memberInputs[0].Focus()
}

func (m *model) openTaskEditForm(selected *task) {
	if selected == nil {
		return
	}
	m.overlay = overlayTask
	m.formCursor = 0
	m.editingTaskID = selected.ID
	for i := range m.taskInputs {
		m.taskInputs[i].SetValue("")
		m.taskInputs[i].Blur()
	}
	m.taskInputs[0].SetValue(selected.Title)
	m.taskInputs[1].SetValue(m.memberName(selected.MemberID))
	m.taskInputs[2].SetValue(selected.Category)
	m.taskInputs[3].SetValue(selected.Priority)
	m.taskInputs[4].SetValue(strings.Join(selected.Tags, ","))
	m.taskInputs[5].SetValue(selected.DueDate)
	m.taskInputs[6].SetValue(strings.Join(selected.Comments, " | "))
	m.refreshMemberSuggestions()
	m.taskInputs[0].Focus()
}

func (m *model) openActionMenu() {
	m.overlay = overlayActions
	m.formCursor = 0
}

func (m *model) openSettings() {
	m.overlay = overlaySettings
	m.formCursor = 0
}

func (m *model) openProjectSwitcher() {
	m.overlay = overlayProjects
	m.projectCreate = false
	m.projectLocked = false
	m.formCursor = 0
	m.projectCursor = m.projectIndex(m.currentProject.ID)
}

func (m *model) openProjectCreateForm(locked bool) {
	m.overlay = overlayProjects
	m.projectCreate = true
	m.projectLocked = locked
	m.editingProjectID = ""
	m.formCursor = 0
	for i := range m.projectInputs {
		m.projectInputs[i].SetValue("")
		m.projectInputs[i].Blur()
	}
	m.projectInputs[1].SetValue(string(projectTypeLocal))
	m.projectInputs[0].Focus()
}

func (m *model) openProjectEditForm(project projectRecord) {
	m.overlay = overlayProjects
	m.projectCreate = true
	m.projectLocked = false
	m.editingProjectID = project.ID
	m.formCursor = 0
	for i := range m.projectInputs {
		m.projectInputs[i].SetValue("")
		m.projectInputs[i].Blur()
	}
	m.projectInputs[0].SetValue(project.Name)
	m.projectInputs[1].SetValue(string(project.Type))
	m.projectInputs[2].SetValue(project.RemoteURL)
	m.projectInputs[0].Focus()
}

func (m *model) navigateForm(total int, direction string) {
	if total == 0 {
		return
	}
	if m.overlay == overlayMember {
		for i := range m.memberInputs {
			m.memberInputs[i].Blur()
		}
	}
	if m.overlay == overlayTask {
		for i := range m.taskInputs {
			m.taskInputs[i].Blur()
		}
	}
	if m.overlay == overlayFilter {
		for i := range m.filterInputs {
			m.filterInputs[i].Blur()
		}
	}
	if m.overlay == overlayNote {
		for i := range m.noteInputs {
			m.noteInputs[i].Blur()
		}
		m.noteInput.Blur()
	}
	if m.overlay == overlayProjects {
		for i := range m.projectInputs {
			m.projectInputs[i].Blur()
		}
	}

	switch direction {
	case "tab", "ctrl+j", "down":
		m.formCursor = (m.formCursor + 1) % total
	case "shift+tab", "ctrl+k", "up":
		m.formCursor--
		if m.formCursor < 0 {
			m.formCursor = total - 1
		}
	}

	if m.overlay == overlayMember {
		m.memberInputs[m.formCursor].Focus()
	}
	if m.overlay == overlayTask {
		m.taskInputs[m.formCursor].Focus()
	}
	if m.overlay == overlayFilter {
		m.filterInputs[m.formCursor].Focus()
	}
	if m.overlay == overlayNote {
		if m.formCursor < len(m.noteInputs) {
			m.noteInputs[m.formCursor].Focus()
		} else {
			m.noteInput.Focus()
		}
	}
	if m.overlay == overlayProjects && m.formCursor < len(m.projectInputs) {
		m.projectInputs[m.formCursor].Focus()
	}
}

func (m *model) submitMemberForm() error {
	name := strings.TrimSpace(m.memberInputs[0].Value())
	role := strings.TrimSpace(m.memberInputs[1].Value())
	email := strings.TrimSpace(m.memberInputs[2].Value())
	if name == "" {
		return errors.New("member name is required")
	}
	existing := m.findMemberByName(name)
	if existing != nil && existing.ID != m.editingMemberID {
		return fmt.Errorf("member %q already exists", name)
	}
	if m.editingMemberID != "" {
		for i := range m.state.Members {
			if m.state.Members[i].ID != m.editingMemberID {
				continue
			}
			m.state.Members[i].Name = name
			m.state.Members[i].Role = role
			m.state.Members[i].Email = email
			m.refreshMemberSuggestions()
			return saveState(m.dbPath, m.state)
		}
		return errors.New("member to edit was not found")
	}
	m.state.Members = append(m.state.Members, member{
		ID:    nextID("mem", time.Now()),
		Name:  name,
		Role:  role,
		Email: email,
	})
	m.refreshMemberSuggestions()
	return saveState(m.dbPath, m.state)
}

func (m *model) submitTaskForm() error {
	title := strings.TrimSpace(m.taskInputs[0].Value())
	memberNames := parseCSV(m.taskInputs[1].Value())
	category := strings.TrimSpace(m.taskInputs[2].Value())
	priority := normalizePriority(m.taskInputs[3].Value())
	tags := parseCSV(m.taskInputs[4].Value())
	dueDate, err := normalizeDueInput(m.taskInputs[5].Value())
	comments := splitComments(m.taskInputs[6].Value())

	if title == "" {
		return errors.New("task title is required")
	}
	if err != nil {
		return err
	}
	if priority == "" {
		priority = "medium"
	}

	memberIDs, err := m.ensureMembers(memberNames)
	if err != nil {
		return err
	}
	if len(memberIDs) == 0 {
		memberIDs = []string{""}
	}
	if m.editingTaskID != "" {
		if len(memberIDs) > 1 {
			return errors.New("editing a task supports at most one member")
		}
		for i := range m.state.Tasks {
			if m.state.Tasks[i].ID != m.editingTaskID {
				continue
			}
			memberID := ""
			if len(memberIDs) == 1 {
				memberID = memberIDs[0]
			}
			m.state.Tasks[i].Title = title
			m.state.Tasks[i].MemberID = memberID
			m.state.Tasks[i].Category = category
			m.state.Tasks[i].Priority = priority
			m.state.Tasks[i].Tags = tags
			m.state.Tasks[i].Comments = comments
			m.state.Tasks[i].DueDate = dueDate
			if err := saveState(m.dbPath, m.state); err != nil {
				return err
			}
			m.activeView = viewTasks
			return nil
		}
		return errors.New("task to edit was not found")
	}

	for _, memberID := range memberIDs {
		m.state.Tasks = append(m.state.Tasks, task{
			ID:        nextID("tsk", time.Now()),
			Title:     title,
			MemberID:  memberID,
			Category:  category,
			Priority:  priority,
			Tags:      tags,
			Comments:  comments,
			DueDate:   dueDate,
			Status:    "open",
			CreatedAt: time.Now(),
		})
	}

	if err := saveState(m.dbPath, m.state); err != nil {
		return err
	}
	m.activeView = viewTasks
	m.cursor[viewTasks] = len(m.filteredTasks()) - 1
	return nil
}

func (m *model) submitNoteForm() (int, error) {
	prefixed, err := m.noteBatchInput()
	if err != nil {
		return 0, err
	}
	tasks, err := parseQuickCapture(prefixed, m.state.Members)
	if err != nil {
		return 0, err
	}
	if len(tasks) == 0 {
		return 0, errors.New("quick note did not produce any tasks")
	}
	m.state.Tasks = append(m.state.Tasks, tasks...)
	if err := saveState(m.dbPath, m.state); err != nil {
		return 0, err
	}
	m.activeView = viewTasks
	m.cursor[viewTasks] = len(m.filteredTasks()) - 1
	return len(tasks), nil
}

func (m model) noteBatchInput() (string, error) {
	memberName := strings.TrimSpace(m.noteInputs[0].Value())
	dueDate, err := normalizeDueInput(m.noteInputs[1].Value())
	if err != nil {
		return "", err
	}

	var lines []string
	if memberName != "" {
		lines = append(lines, "@"+memberName)
	}
	if dueDate != "" {
		lines = append(lines, "due:"+dueDate)
	}
	body := strings.TrimSpace(m.noteInput.Value())
	if body != "" {
		lines = append(lines, body)
	}
	return strings.Join(lines, "\n"), nil
}

func (m *model) submitFilterForm() error {
	due, err := normalizeDueInput(m.filterInputs[2].Value())
	if err != nil {
		return err
	}
	m.filter = filterState{
		Text:   strings.TrimSpace(m.filterInputs[0].Value()),
		Member: strings.TrimSpace(m.filterInputs[1].Value()),
		Due:    due,
	}
	for _, view := range []viewMode{viewTasks, viewMembers, viewDates, viewArchive} {
		m.cursor[view] = 0
	}
	return nil
}

func (m *model) submitProjectForm() (projectRecord, error) {
	name := strings.TrimSpace(m.projectInputs[0].Value())
	if name == "" {
		return projectRecord{}, errors.New("project name is required")
	}
	kind := normalizeProjectType(m.projectInputs[1].Value())
	if kind == "" {
		return projectRecord{}, errors.New("project type must be local or remote")
	}
	remoteURL := strings.TrimSpace(m.projectInputs[2].Value())
	if kind == projectTypeRemote && remoteURL == "" {
		return projectRecord{}, errors.New("remote_url is required for remote projects")
	}
	if kind == projectTypeLocal {
		remoteURL = ""
	}
	if m.editingProjectID != "" {
		project, err := updateProjectRecord(m.registryPath, m.editingProjectID, name, kind, remoteURL)
		if err != nil {
			return projectRecord{}, err
		}
		m.projects, _, err = loadProjectRegistry(m.registryPath)
		if err != nil {
			return projectRecord{}, err
		}
		m.projectCursor = m.projectIndex(project.ID)
		if project.ID == m.currentProject.ID {
			m.currentProject = project
		}
		return project, nil
	}
	project, err := createProjectRecord(m.registryPath, m.projectsBaseDir, name, kind, remoteURL)
	if err != nil {
		return projectRecord{}, err
	}
	m.projects, _, err = loadProjectRegistry(m.registryPath)
	if err != nil {
		return projectRecord{}, err
	}
	m.projectCursor = m.projectIndex(project.ID)
	return project, nil
}

func (m *model) activateProject(project projectRecord) error {
	state, err := loadState(project.DBPath)
	if err != nil {
		return fmt.Errorf("load project %q: %w", project.Name, err)
	}
	cfg, err := loadConfig(project.DBPath)
	if err != nil {
		return fmt.Errorf("load project config %q: %w", project.Name, err)
	}
	if err := touchProjectLastOpened(m.registryPath, project.ID, time.Now()); err != nil {
		return fmt.Errorf("update last project: %w", err)
	}
	projects, _, err := loadProjectRegistry(m.registryPath)
	if err != nil {
		return fmt.Errorf("reload projects: %w", err)
	}
	selected, ok := chooseActiveProject(projects, project.ID)
	if !ok {
		return errors.New("project was not found after switching")
	}

	m.dbPath = selected.DBPath
	m.state = state
	m.config = cfg
	m.projects = projects
	m.currentProject = selected
	m.filter = filterState{}
	m.projectCursor = m.projectIndex(selected.ID)
	m.activeView = viewTasks
	for _, view := range []viewMode{viewTasks, viewMembers, viewDates, viewArchive, viewHelp} {
		m.cursor[view] = 0
		m.listOffset[view] = 0
		m.detailScroll[view] = 0
	}
	for i := range m.filterInputs {
		m.filterInputs[i].SetValue("")
	}
	m.refreshMemberSuggestions()
	return nil
}

func (m model) projectIndex(id string) int {
	for i, project := range m.projects {
		if project.ID == id {
			return i
		}
	}
	return 0
}

func (m model) projectFieldCount() int {
	if normalizeProjectType(m.projectInputs[1].Value()) == projectTypeRemote {
		return 3
	}
	return 2
}

func (m *model) clearFilters() {
	m.filter = filterState{}
	for i := range m.filterInputs {
		m.filterInputs[i].SetValue("")
	}
	m.setStatus("Filters cleared.")
}

func (m model) editSelected() model {
	switch m.activeView {
	case viewTasks:
		selected := m.selectedTask()
		if selected == nil {
			m.setStatus("No task selected.")
			return m
		}
		m.openTaskEditForm(selected)
		m.setStatus("Editing task.")
	case viewMembers:
		selected := m.selectedMember()
		if selected == nil {
			m.setStatus("No member selected.")
			return m
		}
		m.openMemberEditForm(selected)
		m.setStatus("Editing member.")
	case viewArchive:
		m.setStatus("Archived tasks can only be restored or deleted.")
	default:
		m.setStatus("Edit is available in Task and Team views.")
	}
	return m
}

func (m model) toggleSelectedTask() model {
	if m.activeView != viewTasks {
		m.setStatus("Done toggle is only available in Task view.")
		return m
	}
	selected := m.selectedTask()
	if selected == nil {
		m.setStatus("No task selected.")
		return m
	}
	for i := range m.state.Tasks {
		if m.state.Tasks[i].ID != selected.ID {
			continue
		}
		if m.state.Tasks[i].Status == "done" {
			m.state.Tasks[i].Status = "open"
			m.setStatus("Task reopened.")
		} else {
			m.state.Tasks[i].Status = "done"
			m.setStatus("Task completed.")
		}
		break
	}
	if err := saveState(m.dbPath, m.state); err != nil {
		m.setStatus("save failed: " + err.Error())
	}
	return m
}

func (m model) archiveSelectedTask() model {
	if m.activeView != viewTasks {
		m.setStatus("Archive is only available in Task view.")
		return m
	}
	selected := m.selectedTask()
	if selected == nil {
		m.setStatus("No task selected.")
		return m
	}
	if selected.Status != "done" {
		m.setStatus("Only completed tasks can be archived.")
		return m
	}
	for i := range m.state.Tasks {
		if m.state.Tasks[i].ID != selected.ID {
			continue
		}
		m.state.Tasks[i].Archived = true
		break
	}
	if err := saveState(m.dbPath, m.state); err != nil {
		m.setStatus("save failed: " + err.Error())
		return m
	}
	m.setStatus("Task archived.")
	m.clampCursor(len(m.filteredTasks()))
	return m
}

func (m model) restoreSelectedTask() model {
	if m.activeView != viewArchive {
		m.setStatus("Restore is only available in Archive view.")
		return m
	}
	selected := m.selectedTask()
	if selected == nil {
		m.setStatus("No archived task selected.")
		return m
	}
	for i := range m.state.Tasks {
		if m.state.Tasks[i].ID != selected.ID {
			continue
		}
		m.state.Tasks[i].Archived = false
		break
	}
	if err := saveState(m.dbPath, m.state); err != nil {
		m.setStatus("save failed: " + err.Error())
		return m
	}
	m.setStatus("Task restored.")
	m.clampCursor(len(m.archivedTasks()))
	return m
}

func (m model) deleteSelected() model {
	switch m.activeView {
	case viewTasks:
		selected := m.selectedTask()
		if selected == nil {
			m.setStatus("No task selected.")
			return m
		}
		m.state.Tasks = slices.DeleteFunc(m.state.Tasks, func(t task) bool { return t.ID == selected.ID })
		if err := saveState(m.dbPath, m.state); err != nil {
			m.setStatus("save failed: " + err.Error())
			return m
		}
		m.setStatus("Task deleted.")
	case viewArchive:
		selected := m.selectedTask()
		if selected == nil {
			m.setStatus("No archived task selected.")
			return m
		}
		m.state.Tasks = slices.DeleteFunc(m.state.Tasks, func(t task) bool { return t.ID == selected.ID })
		if err := saveState(m.dbPath, m.state); err != nil {
			m.setStatus("save failed: " + err.Error())
			return m
		}
		m.setStatus("Archived task deleted.")
	case viewMembers:
		selected := m.selectedMember()
		if selected == nil {
			m.setStatus("No member selected.")
			return m
		}
		open, done := m.memberTaskCounts(selected.ID)
		if open+done > 0 {
			m.setStatus("Delete member blocked: remove or reassign their tasks first.")
			return m
		}
		m.state.Members = slices.DeleteFunc(m.state.Members, func(mem member) bool { return mem.ID == selected.ID })
		m.refreshMemberSuggestions()
		if err := saveState(m.dbPath, m.state); err != nil {
			m.setStatus("save failed: " + err.Error())
			return m
		}
		m.setStatus("Member deleted.")
	case viewDates:
		m.setStatus("Date view is grouped. Delete tasks from Task View.")
	default:
		m.setStatus("Nothing to delete here.")
	}
	return m
}

func (m model) selectedTask() *task {
	if m.activeView != viewTasks && m.activeView != viewArchive {
		return nil
	}
	tasks := m.filteredTasks()
	cursorView := viewTasks
	if m.activeView == viewArchive {
		tasks = m.archivedTasks()
		cursorView = viewArchive
	}
	if len(tasks) == 0 {
		return nil
	}
	idx := min(max(m.cursor[cursorView], 0), len(tasks)-1)
	return &tasks[idx]
}

func (m model) selectedMember() *member {
	if m.activeView != viewMembers {
		return nil
	}
	members := m.filteredMembers()
	sort.Slice(members, func(i, j int) bool {
		return strings.ToLower(members[i].Name) < strings.ToLower(members[j].Name)
	})
	if len(members) == 0 {
		return nil
	}
	idx := min(max(m.cursor[viewMembers], 0), len(members)-1)
	return &members[idx]
}

func (m model) selectedDateGroup() *dateGroup {
	if m.activeView != viewDates {
		return nil
	}
	groups := m.groupedDates()
	if len(groups) == 0 {
		return nil
	}
	idx := min(max(m.cursor[viewDates], 0), len(groups)-1)
	return &groups[idx]
}

func (m model) taskDetail() string {
	selected := m.selectedTask()
	if selected == nil {
		if m.activeView == viewArchive {
			return ui.sectionTitle.Render("Archived Task Details") + "\n" + ui.subtitle.Render("Restore with r or delete with x.")
		}
		return ui.sectionTitle.Render("Task Details") + "\n" + ui.subtitle.Render("Create a task with t or quick capture with n.")
	}

	memberName := fallback(m.memberName(selected.MemberID), "Unassigned")
	tagLine := "none"
	if len(selected.Tags) > 0 {
		tagLine = strings.Join(selected.Tags, ", ")
	}
	commentLine := "none"
	if len(selected.Comments) > 0 {
		commentLine = strings.Join(selected.Comments, "\n- ")
		commentLine = "- " + commentLine
	}

	lines := []string{
		ui.sectionTitle.Render(selected.Title),
		ui.subtitle.Render(strings.ToUpper(fallback(selected.Category, "uncategorized"))),
		"",
		m.detailPair("State", archiveLabel(selected.Archived)),
		m.detailPair("Status", statusChip(selected.Status)),
		m.detailPair("Member", memberBadge(memberName)+"  "+memberName),
		m.detailPair("Priority", priorityChip(selected.Priority)),
		m.detailPair("Due", dueTone(selected.DueDate)),
		m.detailPair("Tags", tagLine),
		"",
		ui.inputLabel.Render("Comments"),
		commentLine,
		"",
		ui.subtitle.Render(fmt.Sprintf("Created %s", selected.CreatedAt.Format("2006-01-02 15:04"))),
	}
	return strings.Join(lines, "\n")
}

func (m model) memberDetail() string {
	selected := m.selectedMember()
	if selected == nil {
		return ui.sectionTitle.Render("Member Details") + "\n" + ui.subtitle.Render("Add a member with m.")
	}

	tasks := m.tasksForMember(selected.ID)
	lines := []string{
		ui.sectionTitle.Render(selected.Name),
		ui.subtitle.Render(strings.ToUpper(fallback(selected.Role, "team member"))),
		"",
		m.detailPair("Role", fallback(selected.Role, "none")),
		m.detailPair("Contact", fallback(selected.Email, "none")),
		m.detailPair("Badge", memberBadge(selected.Name)),
		"",
		ui.inputLabel.Render("Tasks"),
	}
	if len(tasks) == 0 {
		lines = append(lines, ui.subtitle.Render("No tasks assigned yet."))
		return strings.Join(lines, "\n")
	}
	for _, t := range tasks {
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("%s %s", statusChip(t.Status), t.Title))
		lines = append(lines, ui.subtitle.Render(fmt.Sprintf("  %s  •  %s  •  %s", fallback(t.Category, "no category"), priorityChip(t.Priority), dueLabel(t.DueDate))))
		if len(t.Tags) > 0 {
			lines = append(lines, ui.subtitle.Render("  tags: "+strings.Join(t.Tags, ", ")))
		}
		if len(t.Comments) > 0 {
			lines = append(lines, ui.subtitle.Render("  notes: "+strings.Join(t.Comments, " | ")))
		}
	}
	return strings.Join(lines, "\n")
}

func (m model) dateDetail() string {
	selected := m.selectedDateGroup()
	if selected == nil {
		return ui.sectionTitle.Render("Due Date Details") + "\n" + ui.subtitle.Render("No due dates yet. Add tasks with a due date.")
	}

	lines := []string{
		ui.sectionTitle.Render(dueTone(selected.Date)),
		"",
		ui.subtitle.Render(fmt.Sprintf("%d task%s due", len(selected.Tasks), plural(len(selected.Tasks)))),
		"",
	}
	for _, t := range selected.Tasks {
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("%s %s", statusChip(t.Status), t.Title))
		lines = append(lines, ui.subtitle.Render(fmt.Sprintf("  %s  %s  •  %s  •  %s", memberBadge(fallback(m.memberName(t.MemberID), "Unassigned")), fallback(m.memberName(t.MemberID), "Unassigned"), fallback(t.Category, "no category"), priorityChip(t.Priority))))
		if len(t.Tags) > 0 {
			lines = append(lines, ui.subtitle.Render("  tags: "+strings.Join(t.Tags, ", ")))
		}
		if len(t.Comments) > 0 {
			lines = append(lines, ui.subtitle.Render("  notes: "+strings.Join(t.Comments, " | ")))
		}
	}
	return strings.Join(lines, "\n")
}

func (m model) listHint() string {
	switch m.activeView {
	case viewTasks:
		return "j/k move • space mark done • z archive done task • t add task • n quick notes • f filter • p projects"
	case viewMembers:
		return "j/k move • m add member • t add task for selected member • f filter • p projects"
	case viewDates:
		return "j/k move • grouped by due date • full tasks on right • f filter • p projects"
	case viewArchive:
		return "j/k move • r restore • x delete permanently • p projects"
	case viewHelp:
		return "Full manual on the right pane • p projects"
	default:
		return ""
	}
}

func (m model) formLabel(label string, focused bool) string {
	if focused {
		return ui.inputLabelFocus.Render("> " + label)
	}
	return ui.inputLabel.Render(label)
}

func (m model) detailPair(label, value string) string {
	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		ui.inputLabel.Width(10).Render(label),
		" ",
		value,
	)
}

func (m model) resizeEditors() {
	width := min(80, max(42, m.width-16))
	height := min(18, max(10, m.height-10))
	m.noteInput.SetWidth(width)
	m.noteInput.SetHeight(height)
	for i := range m.memberInputs {
		m.memberInputs[i].Width = width - 4
	}
	for i := range m.taskInputs {
		m.taskInputs[i].Width = width - 4
	}
	for i := range m.filterInputs {
		m.filterInputs[i].Width = width - 4
	}
	for i := range m.noteInputs {
		m.noteInputs[i].Width = width - 4
	}
	for i := range m.projectInputs {
		m.projectInputs[i].Width = width - 4
	}
}

func (m model) taskFormPrefill() (defaultMembers, defaultDueDate string) {
	switch m.activeView {
	case viewMembers:
		selected := m.selectedMember()
		if selected != nil {
			return selected.Name, ""
		}
	case viewDates:
		selected := m.selectedDateGroup()
		if selected != nil {
			return "", selected.Date
		}
	}
	return "", ""
}

func (m *model) refreshMemberSuggestions() {
	names := m.memberNames()
	m.taskInputs[1].SetSuggestions(m.memberSuggestionsForValue(m.taskInputs[1].Value(), true))
	m.filterInputs[1].SetSuggestions(m.memberSuggestionsForValue(m.filterInputs[1].Value(), false))
	m.noteInputs[0].SetSuggestions(m.memberSuggestionsForValue(m.noteInputs[0].Value(), false))
	if len(names) == 0 {
		m.taskInputs[1].SetSuggestions(nil)
		m.filterInputs[1].SetSuggestions(nil)
		m.noteInputs[0].SetSuggestions(nil)
	}
}

func (m model) memberNames() []string {
	members := slices.Clone(m.state.Members)
	sort.Slice(members, func(i, j int) bool {
		return strings.ToLower(members[i].Name) < strings.ToLower(members[j].Name)
	})
	names := make([]string, 0, len(members))
	for _, mem := range members {
		names = append(names, mem.Name)
	}
	return names
}

func (m model) memberSuggestionsForValue(value string, multi bool) []string {
	names := m.memberNames()
	if !multi {
		return names
	}

	raw := value
	if strings.TrimSpace(raw) == "" {
		return names
	}

	lastComma := strings.LastIndex(raw, ",")
	if lastComma == -1 {
		return names
	}

	prefix := raw[:lastComma+1]
	segment := strings.TrimSpace(raw[lastComma+1:])
	var suggestions []string
	for _, name := range names {
		if segment == "" || strings.HasPrefix(strings.ToLower(name), strings.ToLower(segment)) {
			candidate := prefix
			if !strings.HasSuffix(candidate, " ") {
				candidate += " "
			}
			candidate += name
			suggestions = append(suggestions, candidate)
		}
	}
	if len(suggestions) == 0 {
		return names
	}
	return suggestions
}

func (m model) tasksForMember(memberID string) []task {
	var out []task
	for _, t := range m.filteredTasks() {
		if t.MemberID == memberID {
			out = append(out, t)
		}
	}
	return out
}

func (m model) memberTaskCounts(memberID string) (open int, done int) {
	for _, t := range m.state.Tasks {
		if t.Archived {
			continue
		}
		if t.MemberID != memberID {
			continue
		}
		if t.Status == "done" {
			done++
		} else {
			open++
		}
	}
	return open, done
}

func (m model) filteredMemberTaskCounts(memberID string) (open int, done int) {
	for _, t := range m.filteredTasks() {
		if t.MemberID != memberID {
			continue
		}
		if t.Status == "done" {
			done++
		} else {
			open++
		}
	}
	return open, done
}

func (m model) sortedTasks() []task {
	tasks := slices.Clone(m.state.Tasks)
	sort.Slice(tasks, func(i, j int) bool {
		left, right := tasks[i], tasks[j]
		if left.Status != right.Status {
			return left.Status < right.Status
		}
		ld := sortableDue(left.DueDate)
		rd := sortableDue(right.DueDate)
		if !ld.Equal(rd) {
			return ld.Before(rd)
		}
		lp := priorityRank(left.Priority)
		rp := priorityRank(right.Priority)
		if lp != rp {
			return lp > rp
		}
		return strings.ToLower(left.Title) < strings.ToLower(right.Title)
	})
	return tasks
}

func (m model) archivedTasks() []task {
	tasks := slices.DeleteFunc(m.sortedTasks(), func(t task) bool {
		return !t.Archived
	})
	if m.filter == (filterState{}) {
		return tasks
	}
	return slices.DeleteFunc(tasks, func(t task) bool {
		return !m.matchesTaskFilters(t)
	})
}

type dateGroup struct {
	Date  string
	Tasks []task
}

func (m model) groupedDates() []dateGroup {
	groupMap := map[string][]task{}
	for _, t := range m.filteredTasks() {
		if strings.TrimSpace(t.DueDate) == "" {
			continue
		}
		groupMap[t.DueDate] = append(groupMap[t.DueDate], t)
	}
	var groups []dateGroup
	for date, tasks := range groupMap {
		groups = append(groups, dateGroup{Date: date, Tasks: tasks})
	}
	sort.Slice(groups, func(i, j int) bool {
		return sortableDue(groups[i].Date).Before(sortableDue(groups[j].Date))
	})
	return groups
}

func (m model) filteredTasks() []task {
	tasks := slices.DeleteFunc(m.sortedTasks(), func(t task) bool {
		return t.Archived
	})
	if m.filter == (filterState{}) {
		return tasks
	}
	return slices.DeleteFunc(tasks, func(t task) bool {
		return !m.matchesTaskFilters(t)
	})
}

func (m model) filteredMembers() []member {
	if m.filter == (filterState{}) {
		return slices.Clone(m.state.Members)
	}
	var out []member
	for _, mem := range m.state.Members {
		if m.filter.Member != "" && !strings.Contains(strings.ToLower(mem.Name), strings.ToLower(m.filter.Member)) {
			continue
		}
		if m.filter.Text != "" {
			hay := strings.ToLower(strings.Join([]string{mem.Name, mem.Role, mem.Email}, " "))
			if strings.Contains(hay, strings.ToLower(m.filter.Text)) {
				out = append(out, mem)
				continue
			}
		}
		if len(m.tasksForMember(mem.ID)) > 0 {
			out = append(out, mem)
		}
	}
	return out
}

func (m model) matchesTaskFilters(t task) bool {
	if m.filter.Member != "" {
		memberName := strings.ToLower(m.memberName(t.MemberID))
		if !strings.Contains(memberName, strings.ToLower(m.filter.Member)) {
			return false
		}
	}
	if m.filter.Due != "" && strings.TrimSpace(t.DueDate) != m.filter.Due {
		return false
	}
	if m.filter.Text != "" {
		hay := []string{
			t.Title,
			t.Category,
			t.Priority,
			t.DueDate,
			m.memberName(t.MemberID),
			strings.Join(t.Tags, " "),
			strings.Join(t.Comments, " "),
		}
		if !strings.Contains(strings.ToLower(strings.Join(hay, " ")), strings.ToLower(m.filter.Text)) {
			return false
		}
	}
	return true
}

func (m model) filterSummary() string {
	parts := []string{}
	if m.filter.Text != "" {
		parts = append(parts, "text="+m.filter.Text)
	}
	if m.filter.Member != "" {
		parts = append(parts, "member="+m.filter.Member)
	}
	if m.filter.Due != "" {
		parts = append(parts, "due="+m.filter.Due)
	}
	if len(parts) == 0 {
		return "No filters active."
	}
	return "Filters active: " + strings.Join(parts, ", ")
}

func (m model) memberName(memberID string) string {
	for _, mem := range m.state.Members {
		if mem.ID == memberID {
			return mem.Name
		}
	}
	return ""
}

func (m model) ensureMembers(names []string) ([]string, error) {
	var ids []string
	for _, name := range names {
		existing := m.findMemberByName(name)
		if existing == nil {
			return nil, fmt.Errorf("member %q does not exist; add them first", name)
		}
		ids = append(ids, existing.ID)
	}
	return ids, nil
}

func (m model) findMemberByName(name string) *member {
	name = strings.TrimSpace(strings.ToLower(name))
	for _, mem := range m.state.Members {
		if strings.ToLower(mem.Name) == name {
			copy := mem
			return &copy
		}
	}
	return nil
}

func loadState(path string) (appState, error) {
	db, err := openStorage(path)
	if err != nil {
		return appState{}, err
	}
	defer db.Close()

	if err := migrateLegacyFiles(db); err != nil {
		return appState{}, err
	}

	state := appState{}

	memberRows, err := db.Query(`SELECT id, name, role, email FROM members ORDER BY rowid`)
	if err != nil {
		return state, err
	}
	defer memberRows.Close()

	for memberRows.Next() {
		var mem member
		if err := memberRows.Scan(&mem.ID, &mem.Name, &mem.Role, &mem.Email); err != nil {
			return state, err
		}
		state.Members = append(state.Members, mem)
	}
	if err := memberRows.Err(); err != nil {
		return state, err
	}

	taskRows, err := db.Query(`SELECT id, title, member_id, category, priority, tags_json, comments_json, due_date, status, archived, created_at FROM tasks ORDER BY rowid`)
	if err != nil {
		return state, err
	}
	defer taskRows.Close()

	for taskRows.Next() {
		var (
			t            task
			tagsJSON     string
			commentsJSON string
			createdAt    string
			archived     bool
		)
		if err := taskRows.Scan(
			&t.ID,
			&t.Title,
			&t.MemberID,
			&t.Category,
			&t.Priority,
			&tagsJSON,
			&commentsJSON,
			&t.DueDate,
			&t.Status,
			&archived,
			&createdAt,
		); err != nil {
			return state, err
		}
		t.Archived = archived
		if tagsJSON != "" {
			if err := json.Unmarshal([]byte(tagsJSON), &t.Tags); err != nil {
				return state, err
			}
		}
		if commentsJSON != "" {
			if err := json.Unmarshal([]byte(commentsJSON), &t.Comments); err != nil {
				return state, err
			}
		}
		if createdAt != "" {
			parsed, err := time.Parse(time.RFC3339Nano, createdAt)
			if err != nil {
				return state, err
			}
			t.CreatedAt = parsed
		}
		state.Tasks = append(state.Tasks, t)
	}
	return state, taskRows.Err()
}

func loadConfig(path string) (appConfig, error) {
	cfg := defaultConfig()
	db, err := openStorage(path)
	if err != nil {
		return cfg, err
	}
	defer db.Close()

	if err := migrateLegacyFiles(db); err != nil {
		return cfg, err
	}

	row := db.QueryRow(`SELECT value FROM config WHERE key = 'left_wheel_mode'`)
	var value string
	switch err := row.Scan(&value); {
	case errors.Is(err, sql.ErrNoRows):
		return cfg, saveConfig(path, cfg)
	case err != nil:
		return cfg, err
	default:
		cfg.LeftWheelMode = value
	}
	if cfg.leftWheelMode() == "" {
		cfg.LeftWheelMode = defaultConfig().LeftWheelMode
	}
	return cfg, nil
}

func saveConfig(path string, cfg appConfig) error {
	db, err := openStorage(path)
	if err != nil {
		return err
	}
	defer db.Close()

	_, err = db.Exec(
		`INSERT INTO config (key, value) VALUES ('left_wheel_mode', ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		cfg.leftWheelMode(),
	)
	return err
}

func saveState(path string, state appState) error {
	db, err := openStorage(path)
	if err != nil {
		return err
	}
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := replaceState(tx, state); err != nil {
		return err
	}
	return tx.Commit()
}

func openStorage(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA foreign_keys = ON;`); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS members (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			role TEXT NOT NULL,
			email TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS tasks (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			member_id TEXT NOT NULL,
			category TEXT NOT NULL,
			priority TEXT NOT NULL,
			tags_json TEXT NOT NULL,
			comments_json TEXT NOT NULL,
			due_date TEXT NOT NULL,
			status TEXT NOT NULL,
			archived INTEGER NOT NULL,
			created_at TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS config (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);
	`); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func migrateLegacyFiles(db *sql.DB) error {
	empty, err := storageIsEmpty(db)
	if err != nil || !empty {
		return err
	}

	statePath := filepath.Join(".", legacyStateFile)
	configPath := filepath.Join(".", legacyConfigFile)

	if _, err := os.Stat(statePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if _, err := os.Stat(configPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	if !fileExists(statePath) && !fileExists(configPath) {
		return nil
	}

	state, err := loadLegacyState(statePath)
	if err != nil {
		return err
	}
	cfg, err := loadLegacyConfig(configPath)
	if err != nil {
		return err
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := replaceState(tx, state); err != nil {
		return err
	}
	if err := replaceConfig(tx, cfg); err != nil {
		return err
	}
	return tx.Commit()
}

func storageIsEmpty(db *sql.DB) (bool, error) {
	var count int
	if err := db.QueryRow(`SELECT
		(SELECT COUNT(*) FROM members) +
		(SELECT COUNT(*) FROM tasks) +
		(SELECT COUNT(*) FROM config)`).Scan(&count); err != nil {
		return false, err
	}
	return count == 0, nil
}

func replaceState(tx *sql.Tx, state appState) error {
	if _, err := tx.Exec(`DELETE FROM members`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM tasks`); err != nil {
		return err
	}

	for _, mem := range state.Members {
		if _, err := tx.Exec(
			`INSERT INTO members (id, name, role, email) VALUES (?, ?, ?, ?)`,
			mem.ID, mem.Name, mem.Role, mem.Email,
		); err != nil {
			return err
		}
	}

	for _, t := range state.Tasks {
		tagsJSON, err := json.Marshal(t.Tags)
		if err != nil {
			return err
		}
		commentsJSON, err := json.Marshal(t.Comments)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(
			`INSERT INTO tasks (
				id, title, member_id, category, priority, tags_json, comments_json, due_date, status, archived, created_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			t.ID,
			t.Title,
			t.MemberID,
			t.Category,
			t.Priority,
			string(tagsJSON),
			string(commentsJSON),
			t.DueDate,
			t.Status,
			t.Archived,
			t.CreatedAt.Format(time.RFC3339Nano),
		); err != nil {
			return err
		}
	}

	return nil
}

func replaceConfig(tx *sql.Tx, cfg appConfig) error {
	if _, err := tx.Exec(`DELETE FROM config`); err != nil {
		return err
	}
	_, err := tx.Exec(`INSERT INTO config (key, value) VALUES ('left_wheel_mode', ?)`, cfg.leftWheelMode())
	return err
}

func loadLegacyState(path string) (appState, error) {
	var state appState
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return appState{}, nil
	}
	if err != nil {
		return state, err
	}
	if len(data) == 0 {
		return state, nil
	}
	err = json.Unmarshal(data, &state)
	return state, err
}

func loadLegacyConfig(path string) (appConfig, error) {
	cfg := defaultConfig()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if len(data) == 0 {
		return cfg, nil
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	if cfg.leftWheelMode() == "" {
		cfg.LeftWheelMode = defaultConfig().LeftWheelMode
	}
	return cfg, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func loadProjectRegistry(path string) ([]projectRecord, string, error) {
	db, err := openProjectRegistry(path)
	if err != nil {
		return nil, "", err
	}
	defer db.Close()

	rows, err := db.Query(`SELECT id, name, type, remote_url, db_path, created_at, last_opened_at FROM projects ORDER BY COALESCE(last_opened_at, created_at) DESC, lower(name) ASC`)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	var projects []projectRecord
	for rows.Next() {
		var (
			project      projectRecord
			projectTypeV string
			createdAt    string
			lastOpenedAt string
		)
		if err := rows.Scan(&project.ID, &project.Name, &projectTypeV, &project.RemoteURL, &project.DBPath, &createdAt, &lastOpenedAt); err != nil {
			return nil, "", err
		}
		project.Type = normalizeProjectType(projectTypeV)
		if project.Type == "" {
			project.Type = projectTypeLocal
		}
		if createdAt != "" {
			project.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
			if err != nil {
				return nil, "", err
			}
		}
		if lastOpenedAt != "" {
			project.LastOpenedAt, err = time.Parse(time.RFC3339Nano, lastOpenedAt)
			if err != nil {
				return nil, "", err
			}
		}
		projects = append(projects, project)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}

	var lastProjectID string
	if err := db.QueryRow(`SELECT value FROM app_meta WHERE key = 'last_project_id'`).Scan(&lastProjectID); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, "", err
	}
	return projects, lastProjectID, nil
}

func bootstrapDefaultProject(registryPath, projectsBaseDir string, projects []projectRecord, lastProjectID string) ([]projectRecord, string, error) {
	if len(projects) > 0 {
		return projects, lastProjectID, nil
	}

	legacyPath := filepath.Join(".", legacyStorageFile)
	if !fileExists(legacyPath) && !fileExists(filepath.Join(".", legacyStateFile)) && !fileExists(filepath.Join(".", legacyConfigFile)) {
		return projects, lastProjectID, nil
	}

	project, err := createProjectRecord(registryPath, projectsBaseDir, "Default Project", projectTypeLocal, "")
	if err != nil {
		return nil, "", err
	}

	var (
		state appState
		cfg   appConfig
	)
	if fileExists(legacyPath) {
		state, err = loadState(legacyPath)
		if err != nil {
			return nil, "", err
		}
		cfg, err = loadConfig(legacyPath)
		if err != nil {
			return nil, "", err
		}
	} else {
		state, err = loadLegacyState(filepath.Join(".", legacyStateFile))
		if err != nil {
			return nil, "", err
		}
		cfg, err = loadLegacyConfig(filepath.Join(".", legacyConfigFile))
		if err != nil {
			return nil, "", err
		}
	}
	if err := saveState(project.DBPath, state); err != nil {
		return nil, "", err
	}
	if err := saveConfig(project.DBPath, cfg); err != nil {
		return nil, "", err
	}
	return loadProjectRegistry(registryPath)
}

func chooseActiveProject(projects []projectRecord, lastProjectID string) (projectRecord, bool) {
	if len(projects) == 0 {
		return projectRecord{}, false
	}
	if lastProjectID != "" {
		for _, project := range projects {
			if project.ID == lastProjectID {
				return project, true
			}
		}
	}
	return projects[0], true
}

func createProjectRecord(registryPath, projectsBaseDir, name string, kind projectType, remoteURL string) (projectRecord, error) {
	if err := os.MkdirAll(projectsBaseDir, 0o755); err != nil {
		return projectRecord{}, err
	}
	now := time.Now()
	project := projectRecord{
		ID:           nextID("prj", now),
		Name:         name,
		Type:         kind,
		RemoteURL:    remoteURL,
		DBPath:       filepath.Join(projectsBaseDir, nextID("db", now)+".sqlite3"),
		CreatedAt:    now,
		LastOpenedAt: now,
	}
	db, err := openProjectRegistry(registryPath)
	if err != nil {
		return projectRecord{}, err
	}
	defer db.Close()

	_, err = db.Exec(
		`INSERT INTO projects (id, name, type, remote_url, db_path, created_at, last_opened_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		project.ID,
		project.Name,
		string(project.Type),
		project.RemoteURL,
		project.DBPath,
		project.CreatedAt.Format(time.RFC3339Nano),
		project.LastOpenedAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return projectRecord{}, err
	}
	if err := saveLastProjectID(db, project.ID); err != nil {
		return projectRecord{}, err
	}
	if err := saveConfig(project.DBPath, defaultConfig()); err != nil {
		return projectRecord{}, err
	}
	return project, nil
}

func updateProjectRecord(registryPath, projectID, name string, kind projectType, remoteURL string) (projectRecord, error) {
	db, err := openProjectRegistry(registryPath)
	if err != nil {
		return projectRecord{}, err
	}
	defer db.Close()

	if _, err := db.Exec(
		`UPDATE projects SET name = ?, type = ?, remote_url = ? WHERE id = ?`,
		name,
		string(kind),
		remoteURL,
		projectID,
	); err != nil {
		return projectRecord{}, err
	}

	rows, _, err := loadProjectRegistry(registryPath)
	if err != nil {
		return projectRecord{}, err
	}
	for _, project := range rows {
		if project.ID == projectID {
			return project, nil
		}
	}
	return projectRecord{}, errors.New("project not found after update")
}

func touchProjectLastOpened(path, projectID string, openedAt time.Time) error {
	db, err := openProjectRegistry(path)
	if err != nil {
		return err
	}
	defer db.Close()

	if _, err := db.Exec(`UPDATE projects SET last_opened_at = ? WHERE id = ?`, openedAt.Format(time.RFC3339Nano), projectID); err != nil {
		return err
	}
	return saveLastProjectID(db, projectID)
}

func openProjectRegistry(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS projects (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			type TEXT NOT NULL,
			remote_url TEXT NOT NULL,
			db_path TEXT NOT NULL,
			created_at TEXT NOT NULL,
			last_opened_at TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS app_meta (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);
	`); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func saveLastProjectID(db *sql.DB, projectID string) error {
	_, err := db.Exec(
		`INSERT INTO app_meta (key, value) VALUES ('last_project_id', ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		projectID,
	)
	return err
}

func normalizeProjectType(raw string) projectType {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case string(projectTypeLocal), "":
		return projectTypeLocal
	case string(projectTypeRemote):
		return projectTypeRemote
	default:
		return ""
	}
}

func projectTypeBadge(kind projectType) string {
	switch kind {
	case projectTypeRemote:
		return "R"
	default:
		return "L"
	}
}

func normalizeDueInput(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", nil
	}
	if parsed, err := time.ParseInLocation(dateLayout, value, time.Now().Location()); err == nil {
		return parsed.Format(dateLayout), nil
	}
	base := time.Now()
	result, err := naturalDateParser.Parse(value, base)
	if err != nil || result == nil {
		return "", errors.New("due date must be YYYY-MM-DD or natural language like tomorrow, next friday, in 3 days")
	}
	parsed := time.Date(result.Time.Year(), result.Time.Month(), result.Time.Day(), 0, 0, 0, 0, base.Location())
	return parsed.Format(dateLayout), nil
}

func parseQuickCapture(input string, members []member) ([]task, error) {
	type defaults struct {
		memberIDs []string
		category  string
		priority  string
		tags      []string
		dueDate   string
	}

	memberIndex := make(map[string]string, len(members))
	for _, mem := range members {
		memberIndex[strings.ToLower(mem.Name)] = mem.ID
	}

	var out []task
	ctx := defaults{priority: "medium"}

	lines := strings.Split(input, "\n")
	for idx, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "//") {
			continue
		}

		if strings.HasPrefix(line, "@") && !strings.HasPrefix(line, "-") {
			names := parseCSV(strings.TrimPrefix(line, "@"))
			ids, err := resolveMembers(names, memberIndex)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", idx+1, err)
			}
			ctx.memberIDs = ids
			continue
		}
		if strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "-") {
			ctx.category = strings.TrimSpace(strings.TrimPrefix(line, "#"))
			continue
		}
		if strings.HasPrefix(line, "!") && !strings.HasPrefix(line, "-") {
			ctx.priority = normalizePriority(strings.TrimPrefix(line, "!"))
			if ctx.priority == "" {
				ctx.priority = "medium"
			}
			continue
		}
		if strings.HasPrefix(strings.ToLower(line), "due:") && !strings.HasPrefix(line, "-") {
			due, err := normalizeDueInput(line[4:])
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", idx+1, err)
			}
			ctx.dueDate = due
			continue
		}
		if strings.HasPrefix(strings.ToLower(line), "tags:") && !strings.HasPrefix(line, "-") {
			ctx.tags = parseCSV(line[5:])
			continue
		}

		if strings.HasPrefix(line, "-") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "-"))
		}

		comment := ""
		if before, after, ok := strings.Cut(line, "//"); ok {
			line = strings.TrimSpace(before)
			comment = strings.TrimSpace(after)
		}

		meta := ctx
		words := strings.Fields(line)
		titleParts := make([]string, 0, len(words))
		for _, word := range words {
			switch {
			case strings.HasPrefix(word, "@"):
				names := parseCSV(strings.TrimPrefix(word, "@"))
				ids, err := resolveMembers(names, memberIndex)
				if err != nil {
					return nil, fmt.Errorf("line %d: %w", idx+1, err)
				}
				meta.memberIDs = ids
			case strings.HasPrefix(word, "#"):
				meta.category = strings.TrimPrefix(word, "#")
			case strings.HasPrefix(word, "!"):
				meta.priority = normalizePriority(strings.TrimPrefix(word, "!"))
			case strings.HasPrefix(strings.ToLower(word), "due:"):
				date, err := normalizeDueInput(word[4:])
				if err != nil {
					return nil, fmt.Errorf("line %d: %w", idx+1, err)
				}
				meta.dueDate = date
			case strings.HasPrefix(strings.ToLower(word), "tags:"):
				meta.tags = parseCSV(word[5:])
			default:
				titleParts = append(titleParts, word)
			}
		}

		title := strings.TrimSpace(strings.Join(titleParts, " "))
		if title == "" {
			return nil, fmt.Errorf("line %d: task title is empty", idx+1)
		}
		if meta.priority == "" {
			meta.priority = "medium"
		}
		memberIDs := meta.memberIDs
		if len(memberIDs) == 0 {
			memberIDs = []string{""}
		}
		for _, memberID := range memberIDs {
			newTask := task{
				ID:        nextID("tsk", time.Now()),
				Title:     title,
				MemberID:  memberID,
				Category:  meta.category,
				Priority:  meta.priority,
				Tags:      slices.Clone(meta.tags),
				DueDate:   meta.dueDate,
				Status:    "open",
				CreatedAt: time.Now(),
			}
			if comment != "" {
				newTask.Comments = []string{comment}
			}
			out = append(out, newTask)
		}
	}

	return out, nil
}

func resolveMembers(names []string, memberIndex map[string]string) ([]string, error) {
	var ids []string
	for _, name := range names {
		id, ok := memberIndex[strings.ToLower(strings.TrimSpace(name))]
		if !ok {
			return nil, fmt.Errorf("member %q does not exist", name)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func notePlaceholder() string {
	return strings.TrimSpace(`
@Ali
#backend
!high
due:2026-03-20
tags:api,release

- Fix token refresh flow // check mobile fallback
due:next friday
- Review deploy checklist @Ali,Sara #ops !urgent
`)
}

func helpManual(width int) string {
	return strings.Join([]string{
		"Manual",
		"",
		"Keybinds",
		manualKeybindTable(width),
		"",
		"Quick Note Capture",
		"Use the top fields to scope the batch, then write plain task lines below.",
		"The selected member or selected due date prefill those scope fields when available.",
		"Use plain note lines and lightweight tokens.",
		"Standalone defaults:",
		"@Ali,Sara sets default assignees for following tasks.",
		"#backend sets default category.",
		"!high sets default priority.",
		"due:2026-03-20 or due:next friday sets default due date.",
		"tags:api,release sets default tags.",
		"",
		"Task lines",
		"Start with '-' for readability, then write the task title.",
		"Inline tokens override defaults on that line only.",
		"// starts a comment captured into the task comments.",
		"",
		"Examples",
		"- Fix login bug @Ali #backend !urgent due:tomorrow tags:bug,auth // verify on iOS",
		"- Draft sprint notes @Sara",
		"Task form and filter fields also accept: tomorrow, next friday, in 3 days, Mar 20.",
		"",
		"Projects",
		"Press p to switch projects or create a new one inside the TUI.",
		"ProMag remembers the last project you opened and restores it on the next launch.",
		"Remote projects currently store remote_url metadata and still use a local cache DB.",
		"",
		"Data",
		"Project registry and project databases live under .promag/ in this project directory.",
		"Legacy promag.sqlite3, promag-data.json, and promag-config.json are imported automatically when present.",
		"Use s to open settings in-app.",
		"Settings uses up/down or tab to switch options, then enter or ctrl+s to save.",
		"Due dates use YYYY-MM-DD.",
		"Task form supports comma-separated members and will create one task per member.",
		"Mouse: click tabs or list rows, wheel to scroll.",
	}, "\n")
}

func manualKeybindTable(width int) string {
	rows := [][2]string{
		{"1 / 2 / 3 / 4 / 5", "Jump to Tasks, Team, Timeline, Archive, Help"},
		{":", "Open action palette"},
		{"tab / shift+tab", "Cycle views"},
		{"h / l", "Previous / next view"},
		{"j / k, arrows", "Move through the active list"},
		{"gg / G", "First / last row"},
		{"mouse wheel", "Scroll the active pane"},
		{"left click", "Select a tab or row"},
		{"M", "Toggle app mouse vs terminal selection"},
		{"m", "Open member form"},
		{"p", "Open project switcher / create project"},
		{"s", "Open settings"},
		{"e", "Edit selected task or member"},
		{"t", "Open task form"},
		{"a", "Context-aware add action"},
		{"f or /", "Open filters"},
		{"F", "Clear all filters"},
		{"tab", "Accept autocomplete or move forward in forms"},
		{"enter", "Save task or apply filters"},
		{"ctrl+s", "Save task, member, note, or filters"},
		{"up/down, ctrl+j/k", "Move between form fields"},
		{"space", "Toggle done in Task View"},
		{"z", "Archive completed task in Task View"},
		{"r", "Restore task in Archive view"},
		{"x", "Delete selected task, archived task, or empty member"},
		{"n", "Open batch note capture"},
		{"?", "Open this help pane"},
		{"q", "Quit"},
	}

	keyWidth := len("Keys")
	actionWidth := len("Action")
	for _, row := range rows {
		if len(row[0]) > keyWidth {
			keyWidth = len(row[0])
		}
		if len(row[1]) > actionWidth {
			actionWidth = len(row[1])
		}
	}

	fullWidth := keyWidth + actionWidth + 7
	if width > 0 && fullWidth > width {
		lines := make([]string, 0, len(rows)*3)
		for _, row := range rows {
			lines = append(lines, ui.inputLabel.Render(row[0]))
			lines = append(lines, "  "+row[1])
			lines = append(lines, "")
		}
		return strings.TrimRight(strings.Join(lines, "\n"), "\n")
	}

	top := "┌" + strings.Repeat("─", keyWidth+2) + "┬" + strings.Repeat("─", actionWidth+2) + "┐"
	sep := "├" + strings.Repeat("─", keyWidth+2) + "┼" + strings.Repeat("─", actionWidth+2) + "┤"
	bottom := "└" + strings.Repeat("─", keyWidth+2) + "┴" + strings.Repeat("─", actionWidth+2) + "┘"
	lines := []string{
		top,
		fmt.Sprintf("│ %-*s │ %-*s │", keyWidth, "Keys", actionWidth, "Action"),
		sep,
	}
	for _, row := range rows {
		lines = append(lines, fmt.Sprintf("│ %-*s │ %-*s │", keyWidth, row[0], actionWidth, row[1]))
	}
	lines = append(lines, bottom)
	return strings.Join(lines, "\n")
}

func (m model) renderMetric(label, value string, accent lipgloss.Color) string {
	return lipgloss.NewStyle().
		MarginLeft(1).
		Padding(0, 1).
		Background(ui.panelAlt).
		Foreground(ui.text).
		Render(
			lipgloss.JoinHorizontal(
				lipgloss.Center,
				ui.metricLabel.Render(strings.ToUpper(label)),
				" ",
				ui.metricValue.Foreground(accent).Render(value),
			),
		)
}

func (m model) renderLogo(width int) string {
	lines := []string{
		"█▀█ █▀█ █▀█ █▀▄▀█ ▄▀█ █▀▀",
		"█▀▀ █▀▄ █▄█ █ ▀ █ █▀█ █▄█",
	}
	rendered := make([]string, 0, len(lines))
	for i, line := range lines {
		style := lipgloss.NewStyle().Width(width).Align(lipgloss.Center)
		if i == 0 {
			style = style.Foreground(ui.text).Bold(true)
		} else {
			style = style.Foreground(ui.accent)
		}
		rendered = append(rendered, style.Render(line))
	}
	return strings.Join(rendered, "\n")
}

func (m model) openTaskCount() int {
	count := 0
	for _, t := range m.state.Tasks {
		if t.Archived {
			continue
		}
		if t.Status != "done" {
			count++
		}
	}
	return count
}

func (m model) doneTaskCount() int {
	count := 0
	for _, t := range m.state.Tasks {
		if t.Archived {
			continue
		}
		if t.Status == "done" {
			count++
		}
	}
	return count
}

func (m model) overdueTaskCount() int {
	today := time.Now().Format(dateLayout)
	count := 0
	for _, t := range m.state.Tasks {
		if t.Archived {
			continue
		}
		if t.Status == "done" || strings.TrimSpace(t.DueDate) == "" {
			continue
		}
		if t.DueDate < today {
			count++
		}
	}
	return count
}

func dueLabel(date string) string {
	if strings.TrimSpace(date) == "" {
		return "No due date"
	}
	t, err := time.Parse(dateLayout, date)
	if err != nil {
		return date
	}
	now := time.Now()
	diff := int(t.Sub(time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())).Hours() / 24)
	switch {
	case diff == 0:
		return fmt.Sprintf("%s  (today)", date)
	case diff == 1:
		return fmt.Sprintf("%s  (tomorrow)", date)
	case diff == -1:
		return fmt.Sprintf("%s  (yesterday)", date)
	case diff > 1:
		return fmt.Sprintf("%s  (%dd)", date, diff)
	default:
		return fmt.Sprintf("%s  (%dd overdue)", date, -diff)
	}
}

func dueTone(date string) string {
	label := dueLabel(date)
	if strings.TrimSpace(date) == "" {
		return ui.subtitle.Render(label)
	}
	t, err := time.Parse(dateLayout, date)
	if err != nil {
		return label
	}
	today := time.Now()
	base := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())
	switch {
	case t.Before(base):
		return lipgloss.NewStyle().Bold(true).Foreground(ui.danger).Render(label)
	case t.Equal(base):
		return lipgloss.NewStyle().Bold(true).Foreground(ui.warn).Render(label)
	default:
		return lipgloss.NewStyle().Bold(true).Foreground(ui.success).Render(label)
	}
}

func sortableDue(date string) time.Time {
	if strings.TrimSpace(date) == "" {
		return time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	t, err := time.Parse(dateLayout, date)
	if err != nil {
		return time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	return t
}

func priorityRank(priority string) int {
	switch normalizePriority(priority) {
	case "urgent":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

func normalizePriority(priority string) string {
	p := strings.TrimSpace(strings.ToLower(priority))
	switch p {
	case "low", "medium", "high", "urgent":
		return p
	case "med":
		return "medium"
	case "crit", "critical":
		return "urgent"
	default:
		return p
	}
}

func splitComments(value string) []string {
	parts := strings.Split(value, "|")
	var comments []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			comments = append(comments, part)
		}
	}
	return comments
}

func parseCSV(value string) []string {
	parts := strings.Split(value, ",")
	var out []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func nextID(prefix string, now time.Time) string {
	return fmt.Sprintf("%s_%d", prefix, now.UnixNano())
}

func statusChip(status string) string {
	style := lipgloss.NewStyle().Bold(true)
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "done":
		return style.Foreground(lipgloss.Color("#7CC5A1")).Render("done")
	default:
		return style.Foreground(lipgloss.Color("#E7B16A")).Render("open")
	}
}

func statusPill(status string) string {
	label := fallback(strings.ToLower(strings.TrimSpace(status)), "open")
	style := lipgloss.NewStyle().
		Bold(true).
		Padding(0, 1)
	switch label {
	case "archived":
		return style.Foreground(ui.subtle).Render("archived")
	case "done":
		return style.Foreground(ui.success).Render("done")
	default:
		return style.Foreground(ui.warn).Render("open")
	}
}

func archiveLabel(archived bool) string {
	if archived {
		return ui.subtitle.Render("archived")
	}
	return ui.subtitle.Render("active")
}

func priorityChip(priority string) string {
	style := lipgloss.NewStyle().Bold(true)
	switch normalizePriority(priority) {
	case "urgent":
		return style.Foreground(lipgloss.Color("#F28B82")).Render("urgent")
	case "high":
		return style.Foreground(lipgloss.Color("#F6C177")).Render("high")
	case "medium":
		return style.Foreground(lipgloss.Color("#8FB8DE")).Render("medium")
	case "low":
		return style.Foreground(lipgloss.Color("#7CC5A1")).Render("low")
	default:
		return style.Foreground(lipgloss.Color("#AEB7C4")).Render(fallback(priority, "none"))
	}
}

func priorityIcon(priority string) string {
	style := lipgloss.NewStyle().Bold(true)
	switch normalizePriority(priority) {
	case "urgent":
		return style.Foreground(lipgloss.Color("#F28B82")).Render("◆")
	case "high":
		return style.Foreground(lipgloss.Color("#F6C177")).Render("▲")
	case "medium":
		return style.Foreground(lipgloss.Color("#8FB8DE")).Render("●")
	case "low":
		return style.Foreground(lipgloss.Color("#7CC5A1")).Render("•")
	default:
		return style.Foreground(lipgloss.Color("#AEB7C4")).Render("•")
	}
}

func priorityPill(priority string) string {
	label := normalizePriority(priority)
	if label == "" {
		label = "none"
	}
	icon := priorityIcon(label)
	style := lipgloss.NewStyle().
		Bold(true).
		Padding(0, 1).
		Foreground(ui.text)
	return style.Render(lipgloss.JoinHorizontal(lipgloss.Center, icon, " ", label))
}

func memberBadge(name string) string {
	style := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(memberColor(name)))
	initials := initials(name)
	if initials == "" {
		initials = "?"
	}
	return style.Render(initials)
}

func memberColor(name string) string {
	palette := []string{"#A7C7E7", "#B8D8BA", "#D8C3A5", "#E7B7A3", "#B6C7E5", "#D4B6D9"}
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.ToLower(name)))
	return palette[int(h.Sum32())%len(palette)]
}

func initials(name string) string {
	parts := strings.Fields(name)
	if len(parts) == 0 {
		return ""
	}
	if len(parts) == 1 {
		r := []rune(parts[0])
		return strings.ToUpper(string(r[:min(2, len(r))]))
	}
	return strings.ToUpper(string([]rune(parts[0])[0]) + string([]rune(parts[1])[0]))
}

func fallback(value, alt string) string {
	if strings.TrimSpace(value) == "" {
		return alt
	}
	return value
}

func truncate(value string, width int) string {
	if width <= 0 {
		return ""
	}
	return ansi.Truncate(value, width, "…")
}

func joinHeaderLine(left, right string, width int) string {
	if width <= 0 {
		return ""
	}
	leftWidth := lipgloss.Width(left)
	rightWidth := lipgloss.Width(right)
	if leftWidth+rightWidth+1 <= width {
		return left + strings.Repeat(" ", width-leftWidth-rightWidth) + right
	}
	if rightWidth >= width {
		return truncate(right, width)
	}
	availableLeft := max(1, width-rightWidth-1)
	left = truncate(left, availableLeft)
	leftWidth = lipgloss.Width(left)
	return left + strings.Repeat(" ", max(1, width-leftWidth-rightWidth)) + right
}

func renderViewport(content string, width, height, offset int) string {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		lines = []string{""}
	}
	maxOffset := max(0, len(lines)-height)
	if offset < 0 {
		offset = 0
	}
	if offset > maxOffset {
		offset = maxOffset
	}

	end := min(len(lines), offset+height)
	view := make([]string, 0, height)
	for _, line := range lines[offset:end] {
		view = append(view, truncate(line, width))
	}
	for len(view) < height {
		view = append(view, "")
	}
	return strings.Join(view, "\n")
}

func detailContentHeight(content string) int {
	if content == "" {
		return 1
	}
	return max(1, lipgloss.Height(content))
}

func placeOverlay(base, overlay string, width, height int) string {
	base = lipgloss.Place(width, height, lipgloss.Left, lipgloss.Top, base)
	baseLines := strings.Split(base, "\n")
	overlayLines := strings.Split(overlay, "\n")
	overlayWidth := ansi.StringWidth(overlayLines[0])
	overlayHeight := len(overlayLines)
	startX := max(0, (width-overlayWidth)/2)
	startY := max(0, (height-overlayHeight)/2)

	for y, line := range overlayLines {
		targetY := startY + y
		if targetY < 0 || targetY >= len(baseLines) {
			continue
		}
		baseLine := lipgloss.PlaceHorizontal(width, lipgloss.Left, baseLines[targetY])
		left := ansi.Cut(baseLine, 0, startX)
		right := ansi.Cut(baseLine, startX+ansi.StringWidth(line), width)
		baseLines[targetY] = left + line + right
	}

	return strings.Join(baseLines, "\n")
}

func renderMouseDebugOverlay(base string, width, height int) string {
	lines := []string{
		ui.sectionTitle.Render("Mouse Debug"),
		ui.subtitle.Render(fmt.Sprintf("left  (%d,%d)-(%d,%d)", currentLayout.leftZone.X1, currentLayout.leftZone.Y1, currentLayout.leftZone.X2, currentLayout.leftZone.Y2)),
		ui.subtitle.Render(fmt.Sprintf("right (%d,%d)-(%d,%d)", currentLayout.rightZone.X1, currentLayout.rightZone.Y1, currentLayout.rightZone.X2, currentLayout.rightZone.Y2)),
		"",
		ui.eyebrow.Render("Tabs"),
	}
	for _, z := range currentLayout.tabZones {
		lines = append(lines, fmt.Sprintf("%-9s (%d,%d)-(%d,%d)", z.ID, z.X1, z.Y1, z.X2, z.Y2))
	}
	lines = append(lines, "")
	lines = append(lines, ui.eyebrow.Render("Rows"))
	maxRows := min(8, len(currentLayout.rowZones))
	for i := 0; i < maxRows; i++ {
		z := currentLayout.rowZones[i]
		lines = append(lines, fmt.Sprintf("%-12s (%d,%d)-(%d,%d)", z.ID, z.X1, z.Y1, z.X2, z.Y2))
	}
	if len(currentLayout.rowZones) > maxRows {
		lines = append(lines, ui.subtitle.Render(fmt.Sprintf("… %d more rows", len(currentLayout.rowZones)-maxRows)))
	}

	panel := ui.modalFrame.
		Width(min(48, max(36, width/3))).
		Render(strings.Join(lines, "\n"))
	return placeOverlay(base, panel, width, height)
}

func logMouseEvent(format string, args ...any) {
	if mouseDebugLogPath == "" {
		return
	}
	f, err := os.OpenFile(mouseDebugLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = fmt.Fprintf(f, "%s %s\n", time.Now().Format(time.RFC3339Nano), fmt.Sprintf(format, args...))
}

func gSummary(tasks []task, m model) string {
	var parts []string
	for i, t := range tasks {
		if i == 3 {
			parts = append(parts, "…")
			break
		}
		parts = append(parts, fmt.Sprintf("%s:%s", fallback(m.memberName(t.MemberID), "unassigned"), truncate(t.Title, 16)))
	}
	return strings.Join(parts, "  •  ")
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func inZone(x, y int, z zone) bool {
	return x >= z.X1 && x <= z.X2 && y >= z.Y1 && y <= z.Y2
}

func detectTerminalSize() (int, int, bool) {
	width, height, err := term.GetSize(os.Stdout.Fd())
	if err != nil || width <= 0 || height <= 0 {
		return 0, 0, false
	}
	return width, height, true
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
