package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
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
			{ID: "mem-1", Name: "Ali", Role: "Manager", Email: "ali@example.com", Version: 1},
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
				Version:   1,
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

func TestExportImportPreservesCollaborationMetadata(t *testing.T) {
	dir := t.TempDir()
	registryPath := filepath.Join(dir, storageDir, registryFile)
	projectsBaseDir := filepath.Join(dir, storageDir, projectsDir)
	project, err := createProjectRecord(registryPath, projectsBaseDir, "Collab", projectTypeLocal, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := createTask(project.DBPath, task{Title: "Coordinate release", Priority: "medium", Status: "open"}, "actor-1"); err != nil {
		t.Fatal(err)
	}

	projects, lastProjectID, err := loadProjectRegistry(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	exportPath := filepath.Join(dir, "collab.json")
	if _, err := exportProjectBundle(projects, lastProjectID, "Collab", exportPath); err != nil {
		t.Fatal(err)
	}

	projects, _, err = loadProjectRegistry(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	imported, err := importProjectBundle(registryPath, projectsBaseDir, projects, exportPath, "Collab Restored")
	if err != nil {
		t.Fatal(err)
	}
	collaborators, err := loadCollaborators(imported.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	if !hasCollaborator(collaborators, "actor-1") {
		t.Fatalf("imported collaborators = %#v, want actor-1", collaborators)
	}
	events, err := loadActivityLog(imported.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Action != "task.created" {
		t.Fatalf("imported activity = %#v, want one task.created event", events)
	}
}

func TestOpenStorageMigratesOldSchemaAndCreatesBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "project.sqlite3")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		CREATE TABLE members (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			role TEXT NOT NULL,
			email TEXT NOT NULL
		);
		CREATE TABLE tasks (
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
		CREATE TABLE config (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);
		INSERT INTO members (id, name, role, email) VALUES ('mem-old', 'Sara', 'Lead', 'sara@example.com');
		INSERT INTO tasks (
			id, title, member_id, category, priority, tags_json, comments_json, due_date, status, archived, created_at
		) VALUES (
			'tsk-old', 'Review migration', 'mem-old', 'backend', 'medium', '[]', '[]', '2026-05-21', 'open', 0, '2026-05-17T08:00:00Z'
		);
	`)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	migrated, err := openStorage(path)
	if err != nil {
		t.Fatal(err)
	}
	defer migrated.Close()

	backups, err := filepath.Glob(path + ".bak-*")
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 1 {
		t.Fatalf("backup count = %d, want 1", len(backups))
	}
	for _, table := range []string{"members", "tasks"} {
		for _, column := range []string{"version", "updated_at", "updated_by"} {
			ok, err := columnExists(migrated, table, column)
			if err != nil {
				t.Fatal(err)
			}
			if !ok {
				t.Fatalf("%s.%s was not migrated", table, column)
			}
		}
	}

	state, err := loadState(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Members) != 1 || state.Members[0].Version != 1 {
		t.Fatalf("migrated members = %#v, want one versioned member", state.Members)
	}
	if len(state.Tasks) != 1 || state.Tasks[0].Version != 1 {
		t.Fatalf("migrated tasks = %#v, want one versioned task", state.Tasks)
	}
	backups, err = filepath.Glob(path + ".bak-*")
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 1 {
		t.Fatalf("backup count after second open = %d, want 1", len(backups))
	}
}

func TestUpdateTaskDetectsStaleVersionAndLogsActivity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "project.sqlite3")
	task, err := createTask(path, task{
		Title:     "Write API plan",
		Priority:  "medium",
		Status:    "open",
		CreatedAt: time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC),
	}, "actor-1")
	if err != nil {
		t.Fatal(err)
	}

	updated := task
	updated.Title = "Write collaboration API plan"
	updated, err = updateTask(path, updated, task.Version, "actor-1")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Version != 2 {
		t.Fatalf("updated version = %d, want 2", updated.Version)
	}

	task.Title = "Stale title"
	if _, err := updateTask(path, task, task.Version, "actor-2"); !errors.Is(err, errVersionConflict) {
		t.Fatalf("stale update error = %v, want errVersionConflict", err)
	}

	state, err := loadState(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := state.Tasks[0].Title; got != "Write collaboration API plan" {
		t.Fatalf("task title = %q, want latest title", got)
	}
	if got := state.Tasks[0].Version; got != 2 {
		t.Fatalf("task version = %d, want 2", got)
	}

	events, err := loadActivityLog(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("activity event count = %d, want 2", len(events))
	}
	if events[0].Action != "task.created" || events[1].Action != "task.updated" {
		t.Fatalf("activity actions = %q, %q", events[0].Action, events[1].Action)
	}
}

func TestProjectServerRequiresAuthAndHandlesTaskConflicts(t *testing.T) {
	dir := t.TempDir()
	registryPath := filepath.Join(dir, storageDir, registryFile)
	projectsBaseDir := filepath.Join(dir, storageDir, projectsDir)
	project, err := createProjectRecord(registryPath, projectsBaseDir, "Remote Ops", projectTypeLocal, "")
	if err != nil {
		t.Fatal(err)
	}
	handler := newProjectServer(project, "secret")

	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/state", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d, want %d", unauthorized.Code, http.StatusUnauthorized)
	}

	createBody := mustJSON(t, taskWriteRequest{
		Task: task{Title: "Ship server mode", Priority: "high", Status: "open"},
	})
	createResp := serveJSONRequest(t, handler, http.MethodPost, "/tasks", createBody)
	if createResp.Code != http.StatusCreated {
		t.Fatalf("create status = %d body = %s", createResp.Code, createResp.Body.String())
	}
	var created task
	if err := json.Unmarshal(createResp.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Version != 1 || created.UpdatedBy != "manager-1" {
		t.Fatalf("created task metadata = version %d updated_by %q", created.Version, created.UpdatedBy)
	}

	updatedTask := created
	updatedTask.Title = "Ship authenticated server mode"
	updateBody := mustJSON(t, taskWriteRequest{Task: updatedTask, ExpectedVersion: created.Version})
	updateResp := serveJSONRequest(t, handler, http.MethodPatch, "/tasks/"+created.ID, updateBody)
	if updateResp.Code != http.StatusOK {
		t.Fatalf("update status = %d body = %s", updateResp.Code, updateResp.Body.String())
	}
	var updated task
	if err := json.Unmarshal(updateResp.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Version != 2 {
		t.Fatalf("updated version = %d, want 2", updated.Version)
	}

	partialBody := mustJSON(t, taskWriteRequest{
		Task:            task{Title: "Ship partial task patch", Priority: "medium"},
		ExpectedVersion: updated.Version,
	})
	partialResp := serveJSONRequest(t, handler, http.MethodPatch, "/tasks/"+created.ID, partialBody)
	if partialResp.Code != http.StatusOK {
		t.Fatalf("partial update status = %d body = %s", partialResp.Code, partialResp.Body.String())
	}
	var partial task
	if err := json.Unmarshal(partialResp.Body.Bytes(), &partial); err != nil {
		t.Fatal(err)
	}
	if partial.CreatedAt.IsZero() || !partial.CreatedAt.Equal(created.CreatedAt) {
		t.Fatalf("partial update created_at = %v, want preserved %v", partial.CreatedAt, created.CreatedAt)
	}

	staleResp := serveJSONRequest(t, handler, http.MethodPatch, "/tasks/"+created.ID, updateBody)
	if staleResp.Code != http.StatusConflict {
		t.Fatalf("stale update status = %d body = %s", staleResp.Code, staleResp.Body.String())
	}

	stateResp := serveJSONRequest(t, handler, http.MethodGet, "/state", nil)
	if stateResp.Code != http.StatusOK {
		t.Fatalf("state status = %d body = %s", stateResp.Code, stateResp.Body.String())
	}
	var state stateResponse
	if err := json.Unmarshal(stateResp.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if len(state.State.Tasks) != 1 || state.State.Tasks[0].Title != "Ship partial task patch" {
		t.Fatalf("state tasks = %#v", state.State.Tasks)
	}
	if len(state.ActivityLog) != 3 {
		t.Fatalf("activity count = %d, want 3", len(state.ActivityLog))
	}
}

func serveJSONRequest(t *testing.T, handler http.Handler, method, path string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("X-ProMag-Actor", "manager-1")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	return resp
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func hasCollaborator(collaborators []collaborator, id string) bool {
	for _, collab := range collaborators {
		if collab.ID == id {
			return true
		}
	}
	return false
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
