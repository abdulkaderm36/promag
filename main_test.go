package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestSortedTasksShowsOpenTasksBeforeCompletedByDefault(t *testing.T) {
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	m := model{
		config: defaultConfig(),
		state: appState{Tasks: []task{
			{ID: "done-sooner", Title: "Done sooner", Status: "done", DueDate: "2026-05-01", Priority: "urgent", CreatedAt: now},
			{ID: "open-later", Title: "Open later", Status: "open", DueDate: "2026-05-10", Priority: "low", CreatedAt: now.Add(time.Hour)},
			{ID: "open-sooner", Title: "Open sooner", Status: "open", DueDate: "2026-05-02", Priority: "medium", CreatedAt: now.Add(2 * time.Hour)},
		}},
	}

	got := m.sortedTasks()
	gotIDs := taskIDs(got)
	want := []string{"open-sooner", "open-later", "done-sooner"}
	if !equalStrings(gotIDs, want) {
		t.Fatalf("sorted task IDs = %v, want %v", gotIDs, want)
	}
}

func TestSortedTasksUsesConfiguredSecondarySort(t *testing.T) {
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	m := model{
		config: appConfig{TaskSortMode: string(taskSortPriority), LeftWheelMode: "scroll_list"},
		state: appState{Tasks: []task{
			{ID: "open-low", Title: "Open low", Status: "open", DueDate: "2026-05-01", Priority: "low", CreatedAt: now},
			{ID: "done-urgent", Title: "Done urgent", Status: "done", DueDate: "2026-05-01", Priority: "urgent", CreatedAt: now},
			{ID: "open-urgent", Title: "Open urgent", Status: "open", DueDate: "2026-05-10", Priority: "urgent", CreatedAt: now},
		}},
	}

	got := m.sortedTasks()
	gotIDs := taskIDs(got)
	want := []string{"open-urgent", "open-low", "done-urgent"}
	if !equalStrings(gotIDs, want) {
		t.Fatalf("sorted task IDs = %v, want %v", gotIDs, want)
	}
}

func TestLoadConfigAddsDefaultTaskSortMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "project.sqlite3")
	db, err := openStorage(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO config (key, value) VALUES ('left_wheel_mode', 'move_selection')`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.leftWheelMode() != "move_selection" {
		t.Fatalf("left wheel mode = %q, want move_selection", cfg.leftWheelMode())
	}
	if cfg.taskSortMode() != taskSortDue {
		t.Fatalf("task sort mode = %q, want %q", cfg.taskSortMode(), taskSortDue)
	}

	db, err = openStorage(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var saved string
	if err := db.QueryRow(`SELECT value FROM config WHERE key = 'task_sort_mode'`).Scan(&saved); err != nil {
		t.Fatal(err)
	}
	if saved != string(taskSortDue) {
		t.Fatalf("saved task sort mode = %q, want %q", saved, taskSortDue)
	}
}

func TestOpenTaskFormDefaultsDueDateSevenDaysOut(t *testing.T) {
	m := newModel("", "", projectRecord{}, nil, appState{}, defaultConfig())

	m.openTaskForm("", "")

	want := defaultTaskDueDate()
	if got := m.taskInputs[5].Value(); got != want {
		t.Fatalf("task form due date = %q, want %q", got, want)
	}
}

func TestOpenNoteFormDefaultsDueDateSevenDaysOut(t *testing.T) {
	m := newModel("", "", projectRecord{}, nil, appState{}, defaultConfig())

	m.openNoteForm()

	want := defaultTaskDueDate()
	if got := m.noteInputs[1].Value(); got != want {
		t.Fatalf("note form due date = %q, want %q", got, want)
	}
}

func TestExportImportProjectBundleRoundTrip(t *testing.T) {
	dir := t.TempDir()
	registryPath := filepath.Join(dir, storageDir, registryFile)
	projectsBaseDir := filepath.Join(dir, storageDir, projectsDir)
	project, err := createProjectRecord(registryPath, projectsBaseDir, "Ops", projectTypeLocal, "")
	if err != nil {
		t.Fatal(err)
	}
	createdAt := time.Date(2026, 5, 14, 9, 30, 0, 0, time.UTC)
	state := appState{
		Members: []member{
			{ID: "mem-1", Name: "Ali", Role: "Manager", Email: "ali@example.com"},
		},
		Tasks: []task{
			{
				ID:        "tsk-1",
				Title:     "Prepare launch checklist",
				MemberID:  "mem-1",
				Category:  "ops",
				Priority:  "high",
				Tags:      []string{"launch", "release"},
				Comments:  []string{"confirm owner"},
				DueDate:   "2026-05-20",
				Status:    "open",
				CreatedAt: createdAt,
			},
		},
	}
	cfg := appConfig{LeftWheelMode: "move_selection", TaskSortMode: string(taskSortPriority)}
	if err := saveState(project.DBPath, state); err != nil {
		t.Fatal(err)
	}
	if err := saveConfig(project.DBPath, cfg); err != nil {
		t.Fatal(err)
	}

	projects, lastProjectID, err := loadProjectRegistry(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	exportPath := filepath.Join(dir, "ops-export.json")
	exported, err := exportProjectBundle(projects, lastProjectID, "Ops", exportPath)
	if err != nil {
		t.Fatal(err)
	}
	if exported.ID != project.ID {
		t.Fatalf("exported project ID = %q, want %q", exported.ID, project.ID)
	}

	var bundle projectExportBundle
	data, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &bundle); err != nil {
		t.Fatal(err)
	}
	if bundle.Version != exportVersion {
		t.Fatalf("export version = %d, want %d", bundle.Version, exportVersion)
	}
	if bundle.Project.Name != project.Name {
		t.Fatalf("exported project name = %q, want %q", bundle.Project.Name, project.Name)
	}

	projects, _, err = loadProjectRegistry(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	imported, err := importProjectBundle(registryPath, projectsBaseDir, projects, exportPath, "Restored Ops")
	if err != nil {
		t.Fatal(err)
	}
	if imported.ID == project.ID {
		t.Fatal("imported project reused original project ID")
	}
	if imported.Name != "Restored Ops" {
		t.Fatalf("imported project name = %q, want Restored Ops", imported.Name)
	}

	gotState, err := loadState(imported.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotState, state) {
		t.Fatalf("imported state = %#v, want %#v", gotState, state)
	}
	gotCfg, err := loadConfig(imported.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	if gotCfg.leftWheelMode() != cfg.leftWheelMode() || gotCfg.taskSortMode() != cfg.taskSortMode() {
		t.Fatalf("imported config = %#v, want %#v", gotCfg, cfg)
	}
}

func TestSelectProjectForExportRequiresIDForAmbiguousName(t *testing.T) {
	projects := []projectRecord{
		{ID: "prj-1", Name: "Ops"},
		{ID: "prj-2", Name: "ops"},
	}
	if _, err := selectProjectForExport(projects, "", "Ops"); err == nil {
		t.Fatal("expected ambiguous project name error")
	}
	project, err := selectProjectForExport(projects, "", "prj-2")
	if err != nil {
		t.Fatal(err)
	}
	if project.ID != "prj-2" {
		t.Fatalf("selected project ID = %q, want prj-2", project.ID)
	}
}

func taskIDs(tasks []task) []string {
	ids := make([]string, 0, len(tasks))
	for _, task := range tasks {
		ids = append(ids, task.ID)
	}
	return ids
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
