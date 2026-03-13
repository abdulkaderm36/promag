package main

import (
	"encoding/json"
	"errors"
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
	"github.com/olebedev/when"
)

const (
	appTitle     = "ProMag"
	dateLayout   = "2006-01-02"
	storageFile  = "promag-data.json"
	minLeftWidth = 40
)

var naturalDateParser = when.EN
var ui = newTheme()

type viewMode string

const (
	viewTasks   viewMode = "tasks"
	viewMembers viewMode = "members"
	viewDates   viewMode = "dates"
	viewHelp    viewMode = "help"
)

type overlayMode string

const (
	overlayNone   overlayMode = ""
	overlayMember overlayMode = "member"
	overlayTask   overlayMode = "task"
	overlayNote   overlayMode = "note"
	overlayFilter overlayMode = "filter"
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
	CreatedAt time.Time `json:"created_at"`
}

type appState struct {
	Members []member `json:"members"`
	Tasks   []task   `json:"tasks"`
}

type row struct {
	Title    string
	Subtitle string
	ID       string
}

type model struct {
	dataPath string
	state    appState

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

	memberInputs []textinput.Model
	taskInputs   []textinput.Model
	filterInputs []textinput.Model
	noteInputs   []textinput.Model
	noteInput    textarea.Model
	formCursor   int

	pendingG     bool
	filter       filterState
	detailScroll map[viewMode]int
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
			BorderLeft(true).
			BorderStyle(lipgloss.ThickBorder()).
			BorderForeground(lipgloss.Color("#7FDBB6")).
			Padding(0, 1),
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
		title:        lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F5FAFF")),
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
	path := filepath.Join(".", storageFile)
	state, err := loadState(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load state: %v\n", err)
		os.Exit(1)
	}

	p := tea.NewProgram(
		newModel(path, state),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
		tea.WithMouseAllMotion(),
	)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "run app: %v\n", err)
		os.Exit(1)
	}
}

func newModel(path string, state appState) model {
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

	model := model{
		dataPath:     path,
		state:        state,
		activeView:   viewTasks,
		cursor:       map[viewMode]int{viewTasks: 0, viewMembers: 0, viewDates: 0, viewHelp: 0},
		memberInputs: memberInputs,
		taskInputs:   taskInputs,
		filterInputs: filterInputs,
		noteInputs:   noteInputs,
		noteInput:    noteInput,
		lastStatus:   "Ready. Press ? for the full manual.",
		statusAt:     time.Now(),
		detailScroll: map[viewMode]int{viewTasks: 0, viewMembers: 0, viewDates: 0, viewHelp: 0},
	}
	model.refreshMemberSuggestions()
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
	if msg.Action != tea.MouseActionPress {
		if msg.Button == tea.MouseButtonWheelUp {
			return m.scrollByPointer(msg.X, msg.Y, -1), nil
		}
		if msg.Button == tea.MouseButtonWheelDown {
			return m.scrollByPointer(msg.X, msg.Y, 1), nil
		}
		return m, nil
	}

	if msg.Button == tea.MouseButtonWheelUp {
		return m.scrollByPointer(msg.X, msg.Y, -1), nil
	}
	if msg.Button == tea.MouseButtonWheelDown {
		return m.scrollByPointer(msg.X, msg.Y, 1), nil
	}
	if msg.Button != tea.MouseButtonLeft || m.overlay != overlayNone {
		return m, nil
	}

	for _, z := range m.tabZones {
		if inZone(msg.X, msg.Y, z) {
			m.activeView = viewMode(z.ID)
			m.pendingG = false
			return m, nil
		}
	}

	for _, z := range m.rowZones {
		if inZone(msg.X, msg.Y, z) {
			rows := m.rowsForView()
			for idx, r := range rows {
				if r.ID == z.ID {
					m.cursor[m.activeView] = idx
					break
				}
			}
			return m, nil
		}
	}

	return m, nil
}

func (m model) handleOverlayKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.overlay {
	case overlayMember:
		return m.handleMemberForm(msg)
	case overlayTask:
		return m.handleTaskForm(msg)
	case overlayNote:
		return m.handleNoteForm(msg)
	case overlayFilter:
		return m.handleFilterForm(msg)
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
		m.activeView = viewHelp
		return m, nil
	case "tab", "l", "right":
		m.nextView()
		return m, nil
	case "shift+tab", "h", "left":
		m.prevView()
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
			return m, nil
		}
		m.pendingG = true
		return m, nil
	case "G":
		m.cursor[m.activeView] = max(0, len(m.rowsForView())-1)
		m.pendingG = false
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
	case "t":
		m.openTaskForm(m.taskFormPrefill())
		return m, nil
	case "n":
		m.openNoteForm()
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
	}

	m.pendingG = false
	return m, nil
}

func (m model) handleMemberForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.closeOverlay("Member entry cancelled.")
		return m, nil
	case "tab", "shift+tab", "enter", "ctrl+j", "ctrl+k", "up", "down":
		s := msg.String()
		if s == "enter" && m.formCursor == len(m.memberInputs)-1 {
			if err := m.submitMemberForm(); err != nil {
				m.setStatus(err.Error())
				return m, nil
			}
			m.closeOverlay("Member saved.")
			return m, nil
		}
		m.navigateForm(len(m.memberInputs), s)
		return m, nil
	case "ctrl+s":
		if err := m.submitMemberForm(); err != nil {
			m.setStatus(err.Error())
			return m, nil
		}
		m.closeOverlay("Member saved.")
		return m, nil
	}

	var cmd tea.Cmd
	m.memberInputs[m.formCursor], cmd = m.memberInputs[m.formCursor].Update(msg)
	return m, cmd
}

func (m model) handleTaskForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
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
		if m.formCursor == len(m.taskInputs)-1 {
			if err := m.submitTaskForm(); err != nil {
				m.setStatus(err.Error())
				return m, nil
			}
			m.closeOverlay("Task saved.")
			return m, nil
		}
		m.navigateForm(len(m.taskInputs), "tab")
		return m, nil
	case "ctrl+s":
		if err := m.submitTaskForm(); err != nil {
			m.setStatus(err.Error())
			return m, nil
		}
		m.closeOverlay("Task saved.")
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
	case "esc":
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
	case "esc":
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
		if m.formCursor == len(m.filterInputs)-1 {
			if err := m.submitFilterForm(); err != nil {
				m.setStatus(err.Error())
				return m, nil
			}
			m.closeOverlay(m.filterSummary())
			return m, nil
		}
		m.navigateForm(len(m.filterInputs), "tab")
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

func (m model) View() string {
	if m.width == 0 || m.height == 0 {
		return ui.subtitle.Render("loading...")
	}

	m.tabZones = nil
	m.rowZones = nil
	m.leftZone = zone{}
	m.rightZone = zone{}

	header := m.renderHeader()
	m.bodyTop = lipgloss.Height(header)
	status := m.renderStatus()
	bodyHeight := max(8, m.height-lipgloss.Height(header)-lipgloss.Height(status)-1)
	m.bodyHeight = bodyHeight
	body := m.renderBody(bodyHeight)

	screen := lipgloss.JoinVertical(lipgloss.Left, header, body, status)
	if m.overlay != overlayNone {
		screen = screen + "\n" + m.renderOverlay()
	}
	return screen
}

func (m model) renderHeader() string {
	frameWidth := max(20, m.width)
	contentWidth := max(20, frameWidth-ui.headerFrame.GetHorizontalFrameSize())
	views := []viewMode{viewTasks, viewMembers, viewDates, viewHelp}
	labels := map[viewMode]string{
		viewTasks:   "1 Tasks",
		viewMembers: "2 Team",
		viewDates:   "3 Timeline",
		viewHelp:    "4 Guide",
	}

	tabY := 3
	x := 0
	tabParts := make([]string, 0, len(views))
	for _, v := range views {
		label := labels[v]
		style := ui.tabIdle
		if m.activeView == v {
			style = ui.tabActive
		}
		rendered := style.Render(label)
		w := lipgloss.Width(rendered)
		m.tabZones = append(m.tabZones, zone{X1: x, Y1: tabY, X2: x + w - 1, Y2: tabY, ID: string(v)})
		x += w + 1
		tabParts = append(tabParts, rendered)
	}

	stats := []string{
		m.renderMetric("Open", fmt.Sprintf("%d", m.openTaskCount()), ui.warn),
		m.renderMetric("Done", fmt.Sprintf("%d", m.doneTaskCount()), ui.success),
		m.renderMetric("Overdue", fmt.Sprintf("%d", m.overdueTaskCount()), ui.danger),
		m.renderMetric("People", fmt.Sprintf("%d", len(m.state.Members)), ui.borderStrong),
	}
	statsLine := lipgloss.JoinHorizontal(lipgloss.Top, stats...)
	brandLine := lipgloss.JoinHorizontal(
		lipgloss.Center,
		ui.title.Render(appTitle),
		" ",
		ui.subtitle.Render("project board"),
	)
	tabsLine := strings.Join(tabParts, " ")
	if lipgloss.Width(tabsLine) > contentWidth {
		tabsLine = ui.subtitle.Render("1-4 switch views")
	}

	lines := []string{}
	if lipgloss.Width(brandLine)+lipgloss.Width(statsLine)+2 <= contentWidth {
		lines = append(lines, joinHeaderLine(brandLine, statsLine, contentWidth))
	} else {
		lines = append(lines, truncate(brandLine, contentWidth))
		lines = append(lines, truncate(statsLine, contentWidth))
		tabY = 4
	}
	lines = append(lines, "")
	lines = append(lines, tabsLine)
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
	m.leftZone = zone{X1: 0, Y1: m.bodyTop, X2: leftRenderedWidth - 1, Y2: m.bodyTop + bodyHeight - 1, ID: "left"}
	m.rightZone = zone{X1: leftRenderedWidth + gapWidth, Y1: m.bodyTop, X2: leftRenderedWidth + gapWidth + rightRenderedWidth - 1, Y2: m.bodyTop + bodyHeight - 1, ID: "right"}
	return lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right)
}

func (m model) renderList(rows []row, width, height, selected int) string {
	box := ui.panelFrame.Width(width).Height(height)

	title := map[viewMode]string{
		viewTasks:   "Task Queue",
		viewMembers: "Team Directory",
		viewDates:   "Due Timeline",
		viewHelp:    "Manual",
	}[m.activeView]

	lines := []string{
		ui.sectionTitle.Render(title),
		ui.subtitle.Render(m.listHint()),
		"",
	}
	startY := m.bodyTop + 6
	y := startY

	if len(rows) == 0 {
		lines = append(lines, ui.subtitle.Render("No records yet. Use a, m, t, or n."))
		return box.Render(strings.Join(lines, "\n"))
	}

	innerHeight := max(1, height-4)
	maxRows := max(1, innerHeight/2)
	offset := 0
	if selected >= maxRows {
		offset = selected - maxRows + 1
	}

	for idx := offset; idx < len(rows) && idx < offset+maxRows; idx++ {
		r := rows[idx]
		lead := ui.metricLabel.Render(fmt.Sprintf("%02d", idx+1))
		headline := lipgloss.JoinHorizontal(lipgloss.Center, lead, " ", truncate(r.Title, width-12))
		subtitle := ui.subtitle.Render("   " + truncate(r.Subtitle, width-7))
		if idx == selected {
			lines = append(lines, ui.rowSelected.Width(width-6).Render(headline))
			lines = append(lines, subtitle)
		} else {
			lines = append(lines, ui.rowIdle.Width(width-6).Render(headline))
			lines = append(lines, subtitle)
		}
		m.rowZones = append(m.rowZones, zone{X1: 0, Y1: y, X2: width - 1, Y2: y + 1, ID: r.ID})
		y += 2
	}

	return box.Render(strings.Join(lines, "\n"))
}

func (m model) renderDetail(width, height int) string {
	box := ui.panelFrameAlt.Width(width).Height(height)

	content := m.detailContent()
	content = renderViewport(content, max(1, width-4), max(1, height-4), m.detailScroll[m.activeView])
	return box.Render(content)
}

func (m model) detailContent() string {
	switch m.activeView {
	case viewTasks:
		return m.taskDetail()
	case viewMembers:
		return m.memberDetail()
	case viewDates:
		return m.dateDetail()
	case viewHelp:
		return helpManual()
	default:
		return ""
	}
}

func (m model) renderStatus() string {
	frameWidth := max(20, m.width)
	contentWidth := max(20, frameWidth-ui.statusFrame.GetHorizontalFrameSize())
	left := ui.title.Render(truncate(m.lastStatus, max(10, contentWidth-48)))
	filter := ui.subtitle.Render(truncate(m.filterSummary(), max(20, contentWidth/3)))
	right := lipgloss.JoinHorizontal(
		lipgloss.Top,
		ui.keycap.Render("f"),
		" filter ",
		ui.keycap.Render("a"),
		" add ",
		ui.keycap.Render("n"),
		" notes ",
		ui.keycap.Render("?"),
		" guide ",
		ui.keycap.Render("q"),
		" quit",
	)
	row := lipgloss.JoinHorizontal(
		lipgloss.Top,
		left,
		strings.Repeat(" ", max(1, contentWidth-lipgloss.Width(left)-lipgloss.Width(filter)-lipgloss.Width(right)-2)),
		filter,
		"  ",
		right,
	)
	return ui.statusFrame.Width(frameWidth).Render(row)
}

func (m model) renderOverlay() string {
	bg := ui.modalFrame.Width(min(90, max(54, m.width-10)))

	switch m.overlay {
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
		lines = append(lines, ui.subtitle.Render("tab, arrows, or ctrl+j/ctrl+k to move, ctrl+s to save, esc to cancel"))
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
		lines = append(lines, ui.subtitle.Render("Filter by text, member, or due date. ctrl+s applies, ctrl+r clears, esc cancels."))
		lines = append(lines, "")
		labels := []string{"Text", "Member", "Due Date"}
		for i, in := range m.filterInputs {
			lines = append(lines, m.formLabel(labels[i], i == m.formCursor))
			lines = append(lines, in.View())
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
				Title:    fmt.Sprintf("%s  %s  %s", statusChip(t.Status), truncate(t.Title, 26), priorityChip(t.Priority)),
				Subtitle: fmt.Sprintf("%s  •  %s  •  %s", fallback(memberName, "Unassigned"), fallback(t.Category, "no category"), dueLabel(t.DueDate)),
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
	return m
}

func (m model) scrollByPointer(x, y, delta int) model {
	if inZone(x, y, m.rightZone) {
		return m.scrollDetail(delta)
	}
	return m.moveCursor(delta)
}

func (m model) scrollDetail(delta int) model {
	contentHeight := detailContentHeight(m.detailContent())
	visibleHeight := max(1, m.detailHeight)
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

func (m *model) nextView() {
	views := []viewMode{viewTasks, viewMembers, viewDates, viewHelp}
	idx := slices.Index(views, m.activeView)
	m.activeView = views[(idx+1)%len(views)]
}

func (m *model) prevView() {
	views := []viewMode{viewTasks, viewMembers, viewDates, viewHelp}
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
	if status != "" {
		m.setStatus(status)
	}
}

func (m *model) openMemberForm() {
	m.overlay = overlayMember
	m.formCursor = 0
	for i := range m.memberInputs {
		m.memberInputs[i].SetValue("")
		m.memberInputs[i].Blur()
	}
	m.memberInputs[0].Focus()
}

func (m *model) openTaskForm(defaultMembers, defaultDueDate string) {
	m.overlay = overlayTask
	m.formCursor = 0
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
}

func (m *model) submitMemberForm() error {
	name := strings.TrimSpace(m.memberInputs[0].Value())
	role := strings.TrimSpace(m.memberInputs[1].Value())
	email := strings.TrimSpace(m.memberInputs[2].Value())
	if name == "" {
		return errors.New("member name is required")
	}
	if m.findMemberByName(name) != nil {
		return fmt.Errorf("member %q already exists", name)
	}
	m.state.Members = append(m.state.Members, member{
		ID:    nextID("mem", time.Now()),
		Name:  name,
		Role:  role,
		Email: email,
	})
	m.refreshMemberSuggestions()
	return saveState(m.dataPath, m.state)
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

	if err := saveState(m.dataPath, m.state); err != nil {
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
	if err := saveState(m.dataPath, m.state); err != nil {
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
	for _, view := range []viewMode{viewTasks, viewMembers, viewDates} {
		m.cursor[view] = 0
	}
	return nil
}

func (m *model) clearFilters() {
	m.filter = filterState{}
	for i := range m.filterInputs {
		m.filterInputs[i].SetValue("")
	}
	m.setStatus("Filters cleared.")
}

func (m model) toggleSelectedTask() model {
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
	if err := saveState(m.dataPath, m.state); err != nil {
		m.setStatus("save failed: " + err.Error())
	}
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
		if err := saveState(m.dataPath, m.state); err != nil {
			m.setStatus("save failed: " + err.Error())
			return m
		}
		m.setStatus("Task deleted.")
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
		if err := saveState(m.dataPath, m.state); err != nil {
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
	if m.activeView != viewTasks {
		return nil
	}
	tasks := m.filteredTasks()
	if len(tasks) == 0 {
		return nil
	}
	idx := min(max(m.cursor[viewTasks], 0), len(tasks)-1)
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
		return "j/k move • space mark done • t add task • n quick notes • f filter"
	case viewMembers:
		return "j/k move • m add member • t add task for selected member • f filter"
	case viewDates:
		return "j/k move • grouped by due date • full tasks on right • f filter"
	case viewHelp:
		return "Full manual on the right pane"
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
	tasks := m.sortedTasks()
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

func saveState(path string, state appState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
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

func helpManual() string {
	return strings.Join([]string{
		"Manual",
		"",
		"Views",
		"1 / 2 / 3 / 4 jump between Tasks, Members, Due Dates, and Help.",
		"tab / shift+tab or h/l also switch views.",
		"",
		"Navigation",
		"j / k or arrows move through the active list.",
		"gg jumps to the first row. G jumps to the last row.",
		"Mouse wheel scrolls. Left click selects a tab or row.",
		"",
		"Actions",
		"m opens the member form.",
		"t opens the task form. From Member View it prefills the selected member.",
		"a is context-aware: add member in Member View, add task elsewhere.",
		"f or / opens filters. F clears all filters.",
		"tab accepts an autocomplete suggestion in member fields, otherwise it moves forward.",
		"up/down arrows and ctrl+j / ctrl+k move between fields while editing forms.",
		"space toggles a task between open and done in Task View.",
		"x deletes the selected task, or deletes a member with no remaining tasks.",
		"n opens batch note capture with default member and due-date scope fields.",
		"? opens this help pane. q quits.",
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
		"Data",
		"Everything is stored locally in promag-data.json in this project directory.",
		"Due dates use YYYY-MM-DD.",
		"Task form supports comma-separated members and will create one task per member.",
		"Mouse: click tabs or list rows, wheel to scroll.",
	}, "\n")
}

func (m model) renderMetric(label, value string, accent lipgloss.Color) string {
	return lipgloss.NewStyle().MarginLeft(1).Render(
		lipgloss.JoinHorizontal(
			lipgloss.Center,
			ui.metricLabel.Render(label),
			" ",
			ui.metricValue.Foreground(accent).Render(value),
		),
	)
}

func (m model) openTaskCount() int {
	count := 0
	for _, t := range m.state.Tasks {
		if t.Status != "done" {
			count++
		}
	}
	return count
}

func (m model) doneTaskCount() int {
	count := 0
	for _, t := range m.state.Tasks {
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
	if lipgloss.Width(value) <= width {
		return value
	}
	if width <= 1 {
		return value[:width]
	}
	runes := []rune(value)
	if len(runes) > width-1 {
		return string(runes[:width-1]) + "…"
	}
	return value
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
	return len(strings.Split(content, "\n"))
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
