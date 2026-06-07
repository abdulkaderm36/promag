package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
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
	project, err := createProjectRecord(registryPath, projectsBaseDir, "Ops", projectTypeLocal, "", "")
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
	project, err := createProjectRecord(registryPath, projectsBaseDir, "Collab", projectTypeLocal, "", "")
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

func TestUpdateConfigDetectsStaleVersionAndLogsActivity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "project.sqlite3")
	if err := saveConfig(path, defaultConfig()); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Version != 1 {
		t.Fatalf("initial config version = %d, want 1", cfg.Version)
	}

	updatedCfg := cfg
	updatedCfg.TaskSortMode = string(taskSortPriority)
	updated, err := updateConfig(path, updatedCfg, cfg.Version, "manager-1")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Version != 2 || updated.UpdatedBy != "manager-1" {
		t.Fatalf("updated config metadata = version %d updated_by %q", updated.Version, updated.UpdatedBy)
	}

	staleCfg := cfg
	staleCfg.TaskSortMode = string(taskSortTitle)
	if _, err := updateConfig(path, staleCfg, cfg.Version, "manager-2"); !errors.Is(err, errVersionConflict) {
		t.Fatalf("stale config update error = %v, want errVersionConflict", err)
	}

	got, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.taskSortMode() != taskSortPriority || got.Version != 2 {
		t.Fatalf("config after stale update = %#v, want priority version 2", got)
	}
	events, err := loadActivityLog(path)
	if err != nil {
		t.Fatal(err)
	}
	if !hasActivityAction(events, "config.updated") {
		t.Fatalf("activity events = %#v, want config.updated", events)
	}
}

func TestProjectServerRequiresAuthAndHandlesTaskConflicts(t *testing.T) {
	dir := t.TempDir()
	registryPath := filepath.Join(dir, storageDir, registryFile)
	projectsBaseDir := filepath.Join(dir, storageDir, projectsDir)
	project, err := createProjectRecord(registryPath, projectsBaseDir, "Remote Ops", projectTypeLocal, "", "")
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

func TestProjectServerHandlesConfigConflicts(t *testing.T) {
	dir := t.TempDir()
	registryPath := filepath.Join(dir, storageDir, registryFile)
	projectsBaseDir := filepath.Join(dir, storageDir, projectsDir)
	project, err := createProjectRecord(registryPath, projectsBaseDir, "Config Remote", projectTypeLocal, "", "")
	if err != nil {
		t.Fatal(err)
	}
	handler := newProjectServer(project, "secret")

	stateResp := serveJSONRequest(t, handler, http.MethodGet, "/state", nil)
	if stateResp.Code != http.StatusOK {
		t.Fatalf("state status = %d body = %s", stateResp.Code, stateResp.Body.String())
	}
	var state stateResponse
	if err := json.Unmarshal(stateResp.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if state.Config.Version != 1 {
		t.Fatalf("initial config version = %d, want 1", state.Config.Version)
	}

	cfg := state.Config
	cfg.TaskSortMode = string(taskSortPriority)
	updateResp := serveJSONRequest(t, handler, http.MethodPatch, "/config", mustJSON(t, configWriteRequest{
		Config:          cfg,
		ExpectedVersion: state.Config.Version,
	}))
	if updateResp.Code != http.StatusOK {
		t.Fatalf("config update status = %d body = %s", updateResp.Code, updateResp.Body.String())
	}
	var updated appConfig
	if err := json.Unmarshal(updateResp.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Version != 2 || updated.UpdatedBy != "manager-1" {
		t.Fatalf("updated config = %#v, want version 2 manager-1", updated)
	}

	staleCfg := state.Config
	staleCfg.TaskSortMode = string(taskSortTitle)
	staleResp := serveJSONRequest(t, handler, http.MethodPatch, "/config", mustJSON(t, configWriteRequest{
		Config:          staleCfg,
		ExpectedVersion: state.Config.Version,
	}))
	if staleResp.Code != http.StatusConflict {
		t.Fatalf("stale config status = %d body = %s, want conflict", staleResp.Code, staleResp.Body.String())
	}
}

func TestRemoteProjectClientLoadsAndMutatesThroughServer(t *testing.T) {
	dir := t.TempDir()
	registryPath := filepath.Join(dir, storageDir, registryFile)
	projectsBaseDir := filepath.Join(dir, storageDir, projectsDir)
	serverProject, err := createProjectRecord(registryPath, projectsBaseDir, "Server", projectTypeLocal, "", "")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(newProjectServer(serverProject, "secret"))
	defer server.Close()

	t.Setenv(remoteTokenEnv, "secret")
	t.Setenv(remoteActorEnv, "manager-remote")

	remoteProject := projectRecord{
		ID:        "remote-1",
		Name:      "Remote",
		Type:      projectTypeRemote,
		RemoteURL: server.URL,
		DBPath:    filepath.Join(dir, "remote-cache.sqlite3"),
	}
	state, cfg, err := loadProjectData(remoteProject)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Tasks) != 0 {
		t.Fatalf("initial remote tasks = %#v, want none", state.Tasks)
	}
	if cfg.taskSortMode() != taskSortDue {
		t.Fatalf("remote config sort = %q, want default due", cfg.taskSortMode())
	}

	m := newModel("", "", remoteProject, []projectRecord{remoteProject}, state, cfg)
	created, err := m.createTaskRecord(task{Title: "Remote task", Priority: "medium", Status: "open"})
	if err != nil {
		t.Fatal(err)
	}
	if created.UpdatedBy != "manager-remote" {
		t.Fatalf("created updated_by = %q, want manager-remote", created.UpdatedBy)
	}
	if err := m.reloadState(); err != nil {
		t.Fatal(err)
	}
	if len(m.state.Tasks) != 1 || m.state.Tasks[0].Title != "Remote task" {
		t.Fatalf("remote reloaded tasks = %#v", m.state.Tasks)
	}

	cached, err := loadState(remoteProject.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(cached.Tasks) != 1 || cached.Tasks[0].Title != "Remote task" {
		t.Fatalf("cached remote tasks = %#v", cached.Tasks)
	}
}

func TestFetchAndCacheRemoteProjectPersistsCollaborationData(t *testing.T) {
	dir := t.TempDir()
	registryPath := filepath.Join(dir, storageDir, registryFile)
	projectsBaseDir := filepath.Join(dir, storageDir, projectsDir)
	serverProject, err := createProjectRecord(registryPath, projectsBaseDir, "Server Cache", projectTypeLocal, "", "")
	if err != nil {
		t.Fatal(err)
	}
	handler := newProjectServer(serverProject, "secret")
	createResp := serveJSONRequestWithAuth(t, handler, http.MethodPost, "/tasks", mustJSON(t, taskWriteRequest{
		Task: task{Title: "Cached collaboration task", Priority: "medium", Status: "open"},
	}), "secret", "manager-server")
	if createResp.Code != http.StatusCreated {
		t.Fatalf("create task status = %d body = %s", createResp.Code, createResp.Body.String())
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	t.Setenv(remoteTokenEnv, "secret")
	t.Setenv(remoteActorEnv, "manager-cache")
	remoteProject := projectRecord{
		ID:        "remote-cache",
		Name:      "Remote Cache",
		Type:      projectTypeRemote,
		RemoteURL: server.URL,
		DBPath:    filepath.Join(dir, "remote-cache.sqlite3"),
	}
	response, err := fetchAndCacheRemoteProject(remoteProject)
	if err != nil {
		t.Fatal(err)
	}
	if len(response.State.Tasks) != 1 || response.State.Tasks[0].Title != "Cached collaboration task" {
		t.Fatalf("remote response tasks = %#v", response.State.Tasks)
	}
	if !hasCollaborator(response.Collaborators, "manager-cache") {
		t.Fatalf("remote response collaborators = %#v, want manager-cache", response.Collaborators)
	}
	if !hasActivityAction(response.ActivityLog, "task.created") {
		t.Fatalf("remote response activity = %#v, want task.created", response.ActivityLog)
	}

	cachedState, err := loadState(remoteProject.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(cachedState.Tasks) != 1 || cachedState.Tasks[0].Title != "Cached collaboration task" {
		t.Fatalf("cached tasks = %#v", cachedState.Tasks)
	}
	cachedCollaborators, cachedActivity, err := loadProjectCollaborationData(remoteProject)
	if err != nil {
		t.Fatal(err)
	}
	if !hasCollaborator(cachedCollaborators, "manager-cache") {
		t.Fatalf("cached collaborators = %#v, want manager-cache", cachedCollaborators)
	}
	if !hasActivityAction(cachedActivity, "task.created") {
		t.Fatalf("cached activity = %#v, want task.created", cachedActivity)
	}
}

func TestCloudServerCreatesListsAndScopesProjectRoutes(t *testing.T) {
	dir := t.TempDir()
	registryPath := filepath.Join(dir, ".promag-cloud", registryFile)
	projectsBaseDir := filepath.Join(dir, ".promag-cloud", projectsDir)
	handler := newCloudServer(registryPath, projectsBaseDir, "secret")

	createResp := serveJSONRequest(t, handler, http.MethodPost, "/projects", mustJSON(t, projectCreateRequest{Name: "Cloud Ops"}))
	if createResp.Code != http.StatusCreated {
		t.Fatalf("create project status = %d body = %s", createResp.Code, createResp.Body.String())
	}
	var createPayload cloudProjectResponse
	if err := json.Unmarshal(createResp.Body.Bytes(), &createPayload); err != nil {
		t.Fatal(err)
	}
	createdProject := createPayload.Project
	if createdProject.Name != "Cloud Ops" || createdProject.ID == "" {
		t.Fatalf("created project = %#v", createdProject)
	}
	if createPayload.Token == "" {
		t.Fatal("created project token is empty")
	}

	listResp := serveJSONRequest(t, handler, http.MethodGet, "/projects", nil)
	if listResp.Code != http.StatusOK {
		t.Fatalf("list projects status = %d body = %s", listResp.Code, listResp.Body.String())
	}
	var projects []exportedProjectRecord
	if err := json.Unmarshal(listResp.Body.Bytes(), &projects); err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 || projects[0].ID != createdProject.ID {
		t.Fatalf("projects = %#v, want created project", projects)
	}

	taskResp := serveJSONRequest(t, handler, http.MethodPost, "/projects/"+createdProject.ID+"/tasks", mustJSON(t, taskWriteRequest{
		Task: task{Title: "Cloud task", Priority: "medium", Status: "open"},
	}))
	if taskResp.Code != http.StatusCreated {
		t.Fatalf("create cloud task status = %d body = %s", taskResp.Code, taskResp.Body.String())
	}

	stateResp := serveJSONRequest(t, handler, http.MethodGet, "/projects/"+createdProject.ID+"/state", nil)
	if stateResp.Code != http.StatusOK {
		t.Fatalf("cloud state status = %d body = %s", stateResp.Code, stateResp.Body.String())
	}
	var state stateResponse
	if err := json.Unmarshal(stateResp.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if len(state.State.Tasks) != 1 || state.State.Tasks[0].Title != "Cloud task" {
		t.Fatalf("cloud tasks = %#v", state.State.Tasks)
	}
	if state.Project.ID != createdProject.ID {
		t.Fatalf("state project ID = %q, want %q", state.Project.ID, createdProject.ID)
	}

	projectTokenHandler := newCloudServer(registryPath, projectsBaseDir, "secret")
	projectReq := httptest.NewRequest(http.MethodGet, "/projects/"+createdProject.ID+"/state", nil)
	projectReq.Header.Set("Authorization", "Bearer "+createPayload.Token)
	projectResp := httptest.NewRecorder()
	projectTokenHandler.ServeHTTP(projectResp, projectReq)
	if projectResp.Code != http.StatusOK {
		t.Fatalf("project token state status = %d body = %s", projectResp.Code, projectResp.Body.String())
	}

	adminReq := httptest.NewRequest(http.MethodGet, "/projects", nil)
	adminReq.Header.Set("Authorization", "Bearer "+createPayload.Token)
	adminResp := httptest.NewRecorder()
	projectTokenHandler.ServeHTTP(adminResp, adminReq)
	if adminResp.Code != http.StatusUnauthorized {
		t.Fatalf("project token admin status = %d body = %s, want unauthorized", adminResp.Code, adminResp.Body.String())
	}
}

func TestCloudProjectTokensAreScopedAndForwardTaskConflicts(t *testing.T) {
	dir := t.TempDir()
	registryPath := filepath.Join(dir, ".promag-cloud", registryFile)
	projectsBaseDir := filepath.Join(dir, ".promag-cloud", projectsDir)
	firstProject, err := createCloudProject(registryPath, projectsBaseDir, nil, "Cloud One")
	if err != nil {
		t.Fatal(err)
	}
	secondProject, err := createCloudProject(registryPath, projectsBaseDir, nil, "Cloud Two")
	if err != nil {
		t.Fatal(err)
	}
	_, _, firstToken, err := createProjectAccessToken(registryPath, firstProject.ID, "Manager One", "admin-1")
	if err != nil {
		t.Fatal(err)
	}
	_, _, secondToken, err := createProjectAccessToken(registryPath, secondProject.ID, "Manager Two", "admin-1")
	if err != nil {
		t.Fatal(err)
	}
	handler := newCloudServer(registryPath, projectsBaseDir, "admin-secret")

	crossProject := serveJSONRequestWithAuth(t, handler, http.MethodGet, "/projects/"+secondProject.ID+"/state", nil, firstToken, "manager-one")
	if crossProject.Code != http.StatusUnauthorized {
		t.Fatalf("cross-project token status = %d body = %s, want unauthorized", crossProject.Code, crossProject.Body.String())
	}
	wrongToken := serveJSONRequestWithAuth(t, handler, http.MethodGet, "/projects/"+firstProject.ID+"/state", nil, secondToken, "manager-two")
	if wrongToken.Code != http.StatusUnauthorized {
		t.Fatalf("wrong project token status = %d body = %s, want unauthorized", wrongToken.Code, wrongToken.Body.String())
	}

	createResp := serveJSONRequestWithAuth(t, handler, http.MethodPost, "/projects/"+firstProject.ID+"/tasks", mustJSON(t, taskWriteRequest{
		Task: task{Title: "Scoped task", Priority: "medium", Status: "open"},
	}), firstToken, "manager-one")
	if createResp.Code != http.StatusCreated {
		t.Fatalf("create task status = %d body = %s", createResp.Code, createResp.Body.String())
	}
	var created task
	if err := json.Unmarshal(createResp.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	updatedTask := created
	updatedTask.Title = "Scoped task updated"
	updateResp := serveJSONRequestWithAuth(t, handler, http.MethodPatch, "/projects/"+firstProject.ID+"/tasks/"+created.ID, mustJSON(t, taskWriteRequest{
		Task:            updatedTask,
		ExpectedVersion: created.Version,
	}), firstToken, "manager-two")
	if updateResp.Code != http.StatusOK {
		t.Fatalf("update task status = %d body = %s", updateResp.Code, updateResp.Body.String())
	}

	staleTask := created
	staleTask.Title = "Stale overwrite"
	staleResp := serveJSONRequestWithAuth(t, handler, http.MethodPatch, "/projects/"+firstProject.ID+"/tasks/"+created.ID, mustJSON(t, taskWriteRequest{
		Task:            staleTask,
		ExpectedVersion: created.Version,
	}), firstToken, "manager-three")
	if staleResp.Code != http.StatusConflict {
		t.Fatalf("stale update status = %d body = %s, want conflict", staleResp.Code, staleResp.Body.String())
	}

	stateResp := serveJSONRequestWithAuth(t, handler, http.MethodGet, "/projects/"+firstProject.ID+"/state", nil, firstToken, "manager-one")
	if stateResp.Code != http.StatusOK {
		t.Fatalf("state status = %d body = %s", stateResp.Code, stateResp.Body.String())
	}
	var state stateResponse
	if err := json.Unmarshal(stateResp.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if len(state.State.Tasks) != 1 || state.State.Tasks[0].Title != "Scoped task updated" {
		t.Fatalf("cloud scoped tasks = %#v", state.State.Tasks)
	}
}

func TestCloudProjectRoutesForwardMemberConflicts(t *testing.T) {
	dir := t.TempDir()
	registryPath := filepath.Join(dir, ".promag-cloud", registryFile)
	projectsBaseDir := filepath.Join(dir, ".promag-cloud", projectsDir)
	project, err := createCloudProject(registryPath, projectsBaseDir, nil, "Cloud Members")
	if err != nil {
		t.Fatal(err)
	}
	_, _, projectToken, err := createProjectAccessToken(registryPath, project.ID, "Team", "admin-1")
	if err != nil {
		t.Fatal(err)
	}
	handler := newCloudServer(registryPath, projectsBaseDir, "admin-secret")

	createResp := serveJSONRequestWithAuth(t, handler, http.MethodPost, "/projects/"+project.ID+"/members", mustJSON(t, memberWriteRequest{
		Member: member{Name: "Asha Rao", Role: "Manager", Email: "asha@example.test"},
	}), projectToken, "manager-one")
	if createResp.Code != http.StatusCreated {
		t.Fatalf("create member status = %d body = %s", createResp.Code, createResp.Body.String())
	}
	var created member
	if err := json.Unmarshal(createResp.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Version != 1 || created.UpdatedBy != "manager-one" {
		t.Fatalf("created member metadata = version %d updated_by %q", created.Version, created.UpdatedBy)
	}

	updatedMember := created
	updatedMember.Role = "Delivery Lead"
	updateResp := serveJSONRequestWithAuth(t, handler, http.MethodPatch, "/projects/"+project.ID+"/members/"+created.ID, mustJSON(t, memberWriteRequest{
		Member:          updatedMember,
		ExpectedVersion: created.Version,
	}), projectToken, "manager-two")
	if updateResp.Code != http.StatusOK {
		t.Fatalf("update member status = %d body = %s", updateResp.Code, updateResp.Body.String())
	}
	var updated member
	if err := json.Unmarshal(updateResp.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Version != 2 || updated.UpdatedBy != "manager-two" {
		t.Fatalf("updated member metadata = version %d updated_by %q", updated.Version, updated.UpdatedBy)
	}

	staleMember := created
	staleMember.Role = "Stale role"
	staleResp := serveJSONRequestWithAuth(t, handler, http.MethodPatch, "/projects/"+project.ID+"/members/"+created.ID, mustJSON(t, memberWriteRequest{
		Member:          staleMember,
		ExpectedVersion: created.Version,
	}), projectToken, "manager-three")
	if staleResp.Code != http.StatusConflict {
		t.Fatalf("stale member update status = %d body = %s, want conflict", staleResp.Code, staleResp.Body.String())
	}

	stateResp := serveJSONRequestWithAuth(t, handler, http.MethodGet, "/projects/"+project.ID+"/state", nil, projectToken, "manager-one")
	if stateResp.Code != http.StatusOK {
		t.Fatalf("state status = %d body = %s", stateResp.Code, stateResp.Body.String())
	}
	var state stateResponse
	if err := json.Unmarshal(stateResp.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if len(state.State.Members) != 1 || state.State.Members[0].Role != "Delivery Lead" {
		t.Fatalf("cloud scoped members = %#v", state.State.Members)
	}
}

func TestRemoteProjectClientWorksWithCloudProjectURL(t *testing.T) {
	dir := t.TempDir()
	registryPath := filepath.Join(dir, ".promag-cloud", registryFile)
	projectsBaseDir := filepath.Join(dir, ".promag-cloud", projectsDir)
	cloudProject, err := createCloudProject(registryPath, projectsBaseDir, nil, "Cloud Remote")
	if err != nil {
		t.Fatal(err)
	}
	cloudProject, projectToken, err := rotateProjectAccessToken(registryPath, cloudProject.ID)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(newCloudServer(registryPath, projectsBaseDir, "secret"))
	defer server.Close()

	t.Setenv(remoteTokenEnv, projectToken)
	t.Setenv(remoteActorEnv, "manager-cloud")

	remoteProject := projectRecord{
		ID:        "remote-cloud",
		Name:      "Remote Cloud",
		Type:      projectTypeRemote,
		RemoteURL: server.URL + "/projects/" + cloudProject.ID,
		DBPath:    filepath.Join(dir, "cloud-cache.sqlite3"),
	}
	m := newModel("", "", remoteProject, []projectRecord{remoteProject}, appState{}, defaultConfig())
	created, err := m.createTaskRecord(task{Title: "Cloud remote task", Priority: "medium", Status: "open"})
	if err != nil {
		t.Fatal(err)
	}
	if created.UpdatedBy != "manager-cloud" {
		t.Fatalf("created updated_by = %q, want manager-cloud", created.UpdatedBy)
	}
	if err := m.reloadState(); err != nil {
		t.Fatal(err)
	}
	if len(m.state.Tasks) != 1 || m.state.Tasks[0].Title != "Cloud remote task" {
		t.Fatalf("remote cloud tasks = %#v", m.state.Tasks)
	}
	if len(m.collaborators) == 0 || !hasCollaborator(m.collaborators, "manager-cloud") {
		t.Fatalf("remote cloud collaborators = %#v, want manager-cloud", m.collaborators)
	}
}

func TestCloudCollaborationEndToEndBackupRestoreSmoke(t *testing.T) {
	dir := t.TempDir()
	registryPath := filepath.Join(dir, ".promag-cloud", registryFile)
	projectsBaseDir := filepath.Join(dir, ".promag-cloud", projectsDir)
	cloudProject, err := createCloudProject(registryPath, projectsBaseDir, nil, "Cloud Smoke")
	if err != nil {
		t.Fatal(err)
	}
	_, _, projectToken, err := createProjectAccessToken(registryPath, cloudProject.ID, "Shared", "admin-1")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(newCloudServer(registryPath, projectsBaseDir, "admin-secret"))
	defer server.Close()

	baseURL := server.URL + "/projects/" + cloudProject.ID
	firstClient := remoteProjectClient{baseURL: baseURL, token: projectToken, actorID: "manager-one", client: server.Client()}
	secondClient := remoteProjectClient{baseURL: baseURL, token: projectToken, actorID: "manager-two", client: server.Client()}

	createdMember, err := firstClient.createMember(member{Name: "Nora Lee", Role: "Delivery Manager", Email: "nora@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	createdTask, err := secondClient.createTask(task{
		Title:    "Coordinate launch plan",
		MemberID: createdMember.ID,
		Priority: "high",
		Status:   "open",
	})
	if err != nil {
		t.Fatal(err)
	}
	stateBeforeConfig, err := secondClient.state()
	if err != nil {
		t.Fatal(err)
	}
	cfg := stateBeforeConfig.Config
	cfg.TaskSortMode = string(taskSortPriority)
	savedCfg, err := secondClient.saveConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if savedCfg.Version != cfg.Version+1 || savedCfg.UpdatedBy != "manager-two" {
		t.Fatalf("saved config = %#v, want incremented version by manager-two", savedCfg)
	}

	sharedState, err := firstClient.state()
	if err != nil {
		t.Fatal(err)
	}
	if len(sharedState.State.Members) != 1 || sharedState.State.Members[0].ID != createdMember.ID {
		t.Fatalf("shared members = %#v", sharedState.State.Members)
	}
	if len(sharedState.State.Tasks) != 1 || sharedState.State.Tasks[0].ID != createdTask.ID || sharedState.State.Tasks[0].MemberID != createdMember.ID {
		t.Fatalf("shared tasks = %#v", sharedState.State.Tasks)
	}
	if sharedState.Config.taskSortMode() != taskSortPriority || sharedState.Config.UpdatedBy != "manager-two" {
		t.Fatalf("shared config = %#v, want priority updated by manager-two", sharedState.Config)
	}
	for _, actor := range []string{"manager-one", "manager-two"} {
		if !hasCollaborator(sharedState.Collaborators, actor) {
			t.Fatalf("shared collaborators = %#v, want %s", sharedState.Collaborators, actor)
		}
	}
	for _, action := range []string{"token.created", "member.created", "task.created", "config.updated"} {
		if !hasActivityAction(sharedState.ActivityLog, action) {
			t.Fatalf("shared activity = %#v, want %s", sharedState.ActivityLog, action)
		}
	}

	backupPath := filepath.Join(dir, "cloud-smoke-backup.json")
	if err := backupCloudProjects(registryPath, backupPath); err != nil {
		t.Fatal(err)
	}
	backupData, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(backupData, []byte(projectToken)) {
		t.Fatal("cloud smoke backup contained raw project token")
	}

	restoreRegistryPath := filepath.Join(dir, ".promag-cloud-restored", registryFile)
	restoreProjectsBaseDir := filepath.Join(dir, ".promag-cloud-restored", projectsDir)
	if restoredCount, err := restoreCloudProjects(restoreRegistryPath, restoreProjectsBaseDir, backupPath); err != nil {
		t.Fatal(err)
	} else if restoredCount != 1 {
		t.Fatalf("restored count = %d, want 1", restoredCount)
	}
	restoredServer := httptest.NewServer(newCloudServer(restoreRegistryPath, restoreProjectsBaseDir, "admin-secret"))
	defer restoredServer.Close()
	restoredClient := remoteProjectClient{baseURL: restoredServer.URL + "/projects/" + cloudProject.ID, token: projectToken, actorID: "manager-restored", client: restoredServer.Client()}
	restoredState, err := restoredClient.state()
	if err != nil {
		t.Fatal(err)
	}
	if len(restoredState.State.Members) != 1 || restoredState.State.Members[0].ID != createdMember.ID {
		t.Fatalf("restored members = %#v", restoredState.State.Members)
	}
	if len(restoredState.State.Tasks) != 1 || restoredState.State.Tasks[0].ID != createdTask.ID {
		t.Fatalf("restored tasks = %#v", restoredState.State.Tasks)
	}
	if restoredState.Config.taskSortMode() != taskSortPriority || restoredState.Config.Version != savedCfg.Version {
		t.Fatalf("restored config = %#v, want priority version %d", restoredState.Config, savedCfg.Version)
	}
	if !hasActivityAction(restoredState.ActivityLog, "config.updated") {
		t.Fatalf("restored activity = %#v, want config.updated", restoredState.ActivityLog)
	}
}

func TestRemoteProjectClientMapsCloudProjectConflicts(t *testing.T) {
	dir := t.TempDir()
	registryPath := filepath.Join(dir, ".promag-cloud", registryFile)
	projectsBaseDir := filepath.Join(dir, ".promag-cloud", projectsDir)
	cloudProject, err := createCloudProject(registryPath, projectsBaseDir, nil, "Cloud Conflict")
	if err != nil {
		t.Fatal(err)
	}
	_, _, projectToken, err := createProjectAccessToken(registryPath, cloudProject.ID, "Shared", "admin-1")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(newCloudServer(registryPath, projectsBaseDir, "secret"))
	defer server.Close()

	baseURL := server.URL + "/projects/" + cloudProject.ID
	firstClient := remoteProjectClient{baseURL: baseURL, token: projectToken, actorID: "manager-one", client: server.Client()}
	secondClient := remoteProjectClient{baseURL: baseURL, token: projectToken, actorID: "manager-two", client: server.Client()}

	created, err := firstClient.createTask(task{Title: "Remote cloud conflict", Priority: "medium", Status: "open"})
	if err != nil {
		t.Fatal(err)
	}
	secondUpdate := created
	secondUpdate.Title = "Remote cloud conflict updated"
	if _, err := secondClient.updateTask(secondUpdate, created.Version); err != nil {
		t.Fatal(err)
	}

	staleUpdate := created
	staleUpdate.Title = "Remote stale overwrite"
	_, err = firstClient.updateTask(staleUpdate, created.Version)
	if !errors.Is(err, errVersionConflict) {
		t.Fatalf("stale remote update error = %v, want errVersionConflict", err)
	}
}

func TestRemoteProjectClientMapsCloudTaskActionConflicts(t *testing.T) {
	dir := t.TempDir()
	registryPath := filepath.Join(dir, ".promag-cloud", registryFile)
	projectsBaseDir := filepath.Join(dir, ".promag-cloud", projectsDir)
	cloudProject, err := createCloudProject(registryPath, projectsBaseDir, nil, "Cloud Action Conflict")
	if err != nil {
		t.Fatal(err)
	}
	_, _, projectToken, err := createProjectAccessToken(registryPath, cloudProject.ID, "Shared", "admin-1")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(newCloudServer(registryPath, projectsBaseDir, "secret"))
	defer server.Close()

	baseURL := server.URL + "/projects/" + cloudProject.ID
	firstClient := remoteProjectClient{baseURL: baseURL, token: projectToken, actorID: "manager-one", client: server.Client()}
	secondClient := remoteProjectClient{baseURL: baseURL, token: projectToken, actorID: "manager-two", client: server.Client()}

	statusTask, err := firstClient.createTask(task{Title: "Status conflict", Priority: "medium", Status: "open"})
	if err != nil {
		t.Fatal(err)
	}
	if err := secondClient.setTaskStatus(statusTask.ID, "done", statusTask.Version); err != nil {
		t.Fatal(err)
	}
	if err := firstClient.setTaskStatus(statusTask.ID, "open", statusTask.Version); !errors.Is(err, errVersionConflict) {
		t.Fatalf("stale status error = %v, want errVersionConflict", err)
	}

	archiveTask, err := firstClient.createTask(task{Title: "Archive conflict", Priority: "medium", Status: "done"})
	if err != nil {
		t.Fatal(err)
	}
	if err := secondClient.setTaskArchived(archiveTask.ID, true, archiveTask.Version); err != nil {
		t.Fatal(err)
	}
	if err := firstClient.setTaskArchived(archiveTask.ID, false, archiveTask.Version); !errors.Is(err, errVersionConflict) {
		t.Fatalf("stale archive error = %v, want errVersionConflict", err)
	}

	deleteTask, err := firstClient.createTask(task{Title: "Delete conflict", Priority: "medium", Status: "open"})
	if err != nil {
		t.Fatal(err)
	}
	deleteUpdate := deleteTask
	deleteUpdate.Title = "Delete conflict updated"
	if _, err := secondClient.updateTask(deleteUpdate, deleteTask.Version); err != nil {
		t.Fatal(err)
	}
	if err := firstClient.deleteTask(deleteTask.ID, deleteTask.Version); !errors.Is(err, errVersionConflict) {
		t.Fatalf("stale delete error = %v, want errVersionConflict", err)
	}
}

func TestRemoteProjectClientMapsCloudConfigConflicts(t *testing.T) {
	dir := t.TempDir()
	registryPath := filepath.Join(dir, ".promag-cloud", registryFile)
	projectsBaseDir := filepath.Join(dir, ".promag-cloud", projectsDir)
	cloudProject, err := createCloudProject(registryPath, projectsBaseDir, nil, "Cloud Config Conflict")
	if err != nil {
		t.Fatal(err)
	}
	_, _, projectToken, err := createProjectAccessToken(registryPath, cloudProject.ID, "Shared", "admin-1")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(newCloudServer(registryPath, projectsBaseDir, "secret"))
	defer server.Close()

	baseURL := server.URL + "/projects/" + cloudProject.ID
	firstClient := remoteProjectClient{baseURL: baseURL, token: projectToken, actorID: "manager-one", client: server.Client()}
	secondClient := remoteProjectClient{baseURL: baseURL, token: projectToken, actorID: "manager-two", client: server.Client()}

	firstState, err := firstClient.state()
	if err != nil {
		t.Fatal(err)
	}
	secondState, err := secondClient.state()
	if err != nil {
		t.Fatal(err)
	}
	secondCfg := secondState.Config
	secondCfg.TaskSortMode = string(taskSortPriority)
	if _, err := secondClient.saveConfig(secondCfg); err != nil {
		t.Fatal(err)
	}

	firstCfg := firstState.Config
	firstCfg.TaskSortMode = string(taskSortTitle)
	if _, err := firstClient.saveConfig(firstCfg); !errors.Is(err, errVersionConflict) {
		t.Fatalf("stale remote config update error = %v, want errVersionConflict", err)
	}
}

func TestRemoteProjectClientMapsCloudMemberConflicts(t *testing.T) {
	dir := t.TempDir()
	registryPath := filepath.Join(dir, ".promag-cloud", registryFile)
	projectsBaseDir := filepath.Join(dir, ".promag-cloud", projectsDir)
	cloudProject, err := createCloudProject(registryPath, projectsBaseDir, nil, "Cloud Member Conflict")
	if err != nil {
		t.Fatal(err)
	}
	_, _, projectToken, err := createProjectAccessToken(registryPath, cloudProject.ID, "Shared", "admin-1")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(newCloudServer(registryPath, projectsBaseDir, "secret"))
	defer server.Close()

	baseURL := server.URL + "/projects/" + cloudProject.ID
	firstClient := remoteProjectClient{baseURL: baseURL, token: projectToken, actorID: "manager-one", client: server.Client()}
	secondClient := remoteProjectClient{baseURL: baseURL, token: projectToken, actorID: "manager-two", client: server.Client()}

	created, err := firstClient.createMember(member{Name: "Mina Chen", Role: "Manager", Email: "mina@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	secondUpdate := created
	secondUpdate.Role = "Program Lead"
	if _, err := secondClient.updateMember(secondUpdate, created.Version); err != nil {
		t.Fatal(err)
	}

	staleUpdate := created
	staleUpdate.Role = "Stale member role"
	_, err = firstClient.updateMember(staleUpdate, created.Version)
	if !errors.Is(err, errVersionConflict) {
		t.Fatalf("stale remote member update error = %v, want errVersionConflict", err)
	}
}

func TestCloudServerImportsProjectBundle(t *testing.T) {
	dir := t.TempDir()
	registryPath := filepath.Join(dir, ".promag-cloud", registryFile)
	projectsBaseDir := filepath.Join(dir, ".promag-cloud", projectsDir)
	handler := newCloudServer(registryPath, projectsBaseDir, "secret")
	bundle := projectExportBundle{
		Version:    exportVersion,
		ExportedAt: time.Now(),
		Project:    exportedProjectRecord{Name: "Source Ops"},
		Config:     appConfig{LeftWheelMode: "scroll_list", TaskSortMode: string(taskSortPriority)},
		State: appState{Tasks: []task{{
			ID:        "tsk-import",
			Title:     "Imported cloud task",
			Priority:  "high",
			Status:    "open",
			CreatedAt: time.Now(),
			Version:   1,
		}}},
	}

	importResp := serveJSONRequest(t, handler, http.MethodPost, "/projects/import", mustJSON(t, projectImportRequest{Name: "Imported Cloud", Bundle: bundle}))
	if importResp.Code != http.StatusCreated {
		t.Fatalf("import status = %d body = %s", importResp.Code, importResp.Body.String())
	}
	var importPayload cloudProjectResponse
	if err := json.Unmarshal(importResp.Body.Bytes(), &importPayload); err != nil {
		t.Fatal(err)
	}
	imported := importPayload.Project
	if importPayload.Token == "" {
		t.Fatal("imported project token is empty")
	}

	stateResp := serveJSONRequest(t, handler, http.MethodGet, "/projects/"+imported.ID+"/state", nil)
	if stateResp.Code != http.StatusOK {
		t.Fatalf("imported state status = %d body = %s", stateResp.Code, stateResp.Body.String())
	}
	var state stateResponse
	if err := json.Unmarshal(stateResp.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if len(state.State.Tasks) != 1 || state.State.Tasks[0].Title != "Imported cloud task" {
		t.Fatalf("imported cloud tasks = %#v", state.State.Tasks)
	}
	if state.Config.taskSortMode() != taskSortPriority {
		t.Fatalf("imported cloud sort = %q, want priority", state.Config.taskSortMode())
	}
}

func TestProjectAccessTokensCanBeMultipleAndRevoked(t *testing.T) {
	dir := t.TempDir()
	registryPath := filepath.Join(dir, ".promag-cloud", registryFile)
	projectsBaseDir := filepath.Join(dir, ".promag-cloud", projectsDir)
	project, err := createCloudProject(registryPath, projectsBaseDir, nil, "Tokened")
	if err != nil {
		t.Fatal(err)
	}
	_, firstRecord, firstToken, err := createProjectAccessToken(registryPath, project.ID, "Manager One", "admin-1")
	if err != nil {
		t.Fatal(err)
	}
	_, secondRecord, secondToken, err := createProjectAccessToken(registryPath, project.ID, "Manager Two", "admin-1")
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := loadProjectAccessTokens(registryPath, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tokens) != 2 {
		t.Fatalf("tokens = %#v, want two", tokens)
	}

	handler := newCloudServer(registryPath, projectsBaseDir, "admin-secret")
	req := httptest.NewRequest(http.MethodGet, "/projects/"+project.ID+"/state", nil)
	req.Header.Set("Authorization", "Bearer "+firstToken)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("first token status = %d body = %s", resp.Code, resp.Body.String())
	}

	revoked, err := revokeProjectAccessToken(registryPath, firstRecord.ID, "admin-1")
	if err != nil {
		t.Fatal(err)
	}
	if revoked.RevokedAt.IsZero() {
		t.Fatalf("revoked token = %#v, want revoked_at", revoked)
	}

	req = httptest.NewRequest(http.MethodGet, "/projects/"+project.ID+"/state", nil)
	req.Header.Set("Authorization", "Bearer "+firstToken)
	resp = httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("revoked token status = %d body = %s, want unauthorized", resp.Code, resp.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/projects/"+project.ID+"/state", nil)
	req.Header.Set("Authorization", "Bearer "+secondToken)
	resp = httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("second token status = %d body = %s", resp.Code, resp.Body.String())
	}
	if secondRecord.Label != "Manager Two" {
		t.Fatalf("second token label = %q, want Manager Two", secondRecord.Label)
	}
	events, err := loadActivityLog(project.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	if !hasActivityAction(events, "token.created") || !hasActivityAction(events, "token.revoked") {
		t.Fatalf("token activity events = %#v, want token.created and token.revoked", events)
	}
}

func TestCloudProjectTokenAdminHTTPLifecycle(t *testing.T) {
	dir := t.TempDir()
	registryPath := filepath.Join(dir, ".promag-cloud", registryFile)
	projectsBaseDir := filepath.Join(dir, ".promag-cloud", projectsDir)
	project, err := createCloudProject(registryPath, projectsBaseDir, nil, "HTTP Tokens")
	if err != nil {
		t.Fatal(err)
	}
	_, _, projectToken, err := createProjectAccessToken(registryPath, project.ID, "Project Client", "admin-1")
	if err != nil {
		t.Fatal(err)
	}
	handler := newCloudServer(registryPath, projectsBaseDir, "admin-secret")

	projectTokenResp := serveJSONRequestWithAuth(t, handler, http.MethodGet, "/projects/"+project.ID+"/tokens", nil, projectToken, "manager-one")
	if projectTokenResp.Code != http.StatusUnauthorized {
		t.Fatalf("project token list status = %d body = %s, want unauthorized", projectTokenResp.Code, projectTokenResp.Body.String())
	}

	createResp := serveJSONRequestWithAuth(t, handler, http.MethodPost, "/projects/"+project.ID+"/tokens", mustJSON(t, projectTokenCreateRequest{
		Label: "Manager Laptop",
	}), "admin-secret", "admin-1")
	if createResp.Code != http.StatusCreated {
		t.Fatalf("create token status = %d body = %s", createResp.Code, createResp.Body.String())
	}
	var created cloudProjectTokenResponse
	if err := json.Unmarshal(createResp.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Token == "" || created.AccessToken.ID == "" || created.AccessToken.Label != "Manager Laptop" {
		t.Fatalf("created token response = %#v", created)
	}
	if strings.Contains(createResp.Body.String(), "token_hash") || strings.Contains(createResp.Body.String(), hashToken(created.Token)) {
		t.Fatalf("created token response exposed token hash: %s", createResp.Body.String())
	}

	listResp := serveJSONRequestWithAuth(t, handler, http.MethodGet, "/projects/"+project.ID+"/tokens", nil, "admin-secret", "admin-1")
	if listResp.Code != http.StatusOK {
		t.Fatalf("list tokens status = %d body = %s", listResp.Code, listResp.Body.String())
	}
	var listed []exportedProjectAccessToken
	if err := json.Unmarshal(listResp.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 {
		t.Fatalf("listed tokens = %#v, want two", listed)
	}
	if strings.Contains(listResp.Body.String(), "token_hash") || strings.Contains(listResp.Body.String(), hashToken(created.Token)) {
		t.Fatalf("list token response exposed token hash: %s", listResp.Body.String())
	}

	stateReq := httptest.NewRequest(http.MethodGet, "/projects/"+project.ID+"/state", nil)
	stateReq.Header.Set("Authorization", "Bearer "+created.Token)
	stateResp := httptest.NewRecorder()
	handler.ServeHTTP(stateResp, stateReq)
	if stateResp.Code != http.StatusOK {
		t.Fatalf("new token state status = %d body = %s", stateResp.Code, stateResp.Body.String())
	}

	revokeResp := serveJSONRequestWithAuth(t, handler, http.MethodDelete, "/projects/"+project.ID+"/tokens/"+created.AccessToken.ID, nil, "admin-secret", "admin-1")
	if revokeResp.Code != http.StatusOK {
		t.Fatalf("revoke token status = %d body = %s", revokeResp.Code, revokeResp.Body.String())
	}
	var revoked cloudProjectTokenResponse
	if err := json.Unmarshal(revokeResp.Body.Bytes(), &revoked); err != nil {
		t.Fatal(err)
	}
	if revoked.AccessToken.ID != created.AccessToken.ID || revoked.AccessToken.RevokedAt.IsZero() {
		t.Fatalf("revoked token response = %#v", revoked)
	}

	stateReq = httptest.NewRequest(http.MethodGet, "/projects/"+project.ID+"/state", nil)
	stateReq.Header.Set("Authorization", "Bearer "+created.Token)
	stateResp = httptest.NewRecorder()
	handler.ServeHTTP(stateResp, stateReq)
	if stateResp.Code != http.StatusUnauthorized {
		t.Fatalf("revoked token state status = %d body = %s, want unauthorized", stateResp.Code, stateResp.Body.String())
	}
}

func TestCloudProjectTokenAdminHTTPDoesNotRevokeOtherProjectsToken(t *testing.T) {
	dir := t.TempDir()
	registryPath := filepath.Join(dir, ".promag-cloud", registryFile)
	projectsBaseDir := filepath.Join(dir, ".promag-cloud", projectsDir)
	firstProject, err := createCloudProject(registryPath, projectsBaseDir, nil, "HTTP Token One")
	if err != nil {
		t.Fatal(err)
	}
	secondProject, err := createCloudProject(registryPath, projectsBaseDir, nil, "HTTP Token Two")
	if err != nil {
		t.Fatal(err)
	}
	_, secondTokenRecord, secondToken, err := createProjectAccessToken(registryPath, secondProject.ID, "Other Project", "admin-1")
	if err != nil {
		t.Fatal(err)
	}
	handler := newCloudServer(registryPath, projectsBaseDir, "admin-secret")

	revokeResp := serveJSONRequestWithAuth(t, handler, http.MethodDelete, "/projects/"+firstProject.ID+"/tokens/"+secondTokenRecord.ID, nil, "admin-secret", "admin-1")
	if revokeResp.Code != http.StatusNotFound {
		t.Fatalf("cross-project revoke status = %d body = %s, want not found", revokeResp.Code, revokeResp.Body.String())
	}

	stateReq := httptest.NewRequest(http.MethodGet, "/projects/"+secondProject.ID+"/state", nil)
	stateReq.Header.Set("Authorization", "Bearer "+secondToken)
	stateResp := httptest.NewRecorder()
	handler.ServeHTTP(stateResp, stateReq)
	if stateResp.Code != http.StatusOK {
		t.Fatalf("other project token status = %d body = %s, want still active", stateResp.Code, stateResp.Body.String())
	}
}

func TestCloudTokenCLICommands(t *testing.T) {
	dir := t.TempDir()
	bin := buildPromagTestBinary(t, dir)

	dataDir := filepath.Join(dir, "cloud-data")
	createOutput := runPromagCLI(t, bin, "--cloud-create", "--data-dir", dataDir, "Ops")
	if !strings.Contains(createOutput, "Created cloud project \"Ops\"") {
		t.Fatalf("cloud create output = %q", createOutput)
	}
	initialTokenID := extractOutputValue(t, createOutput, "Project token ID: ")
	initialToken := extractOutputValue(t, createOutput, "Project token: ")

	tokenOutput := runPromagCLI(t, bin, "--cloud-token", "Ops", "--token-label", "manager laptop", "--data-dir", dataDir)
	if !strings.Contains(tokenOutput, "Created cloud project token for \"Ops\"") {
		t.Fatalf("cloud token output = %q", tokenOutput)
	}
	labeledTokenID := extractOutputValue(t, tokenOutput, "Project token ID: ")
	labeledToken := extractOutputValue(t, tokenOutput, "Project token: ")

	listOutput := runPromagCLI(t, bin, "--cloud-tokens", "Ops", "--data-dir", dataDir)
	for _, want := range []string{
		initialTokenID + "\tdefault\tactive",
		labeledTokenID + "\tmanager laptop\tactive",
	} {
		if !strings.Contains(listOutput, want) {
			t.Fatalf("token list output = %q, want %q", listOutput, want)
		}
	}

	revokeOutput := runPromagCLI(t, bin, "--cloud-revoke-token", labeledTokenID, "--data-dir", dataDir)
	if !strings.Contains(revokeOutput, "Revoked cloud project token "+labeledTokenID+" (manager laptop)") {
		t.Fatalf("revoke output = %q", revokeOutput)
	}
	listOutput = runPromagCLI(t, bin, "--cloud-tokens", "Ops", "--data-dir", dataDir)
	if !strings.Contains(listOutput, initialTokenID+"\tdefault\tactive") {
		t.Fatalf("token list after revoke = %q, want initial token active", listOutput)
	}
	if !strings.Contains(listOutput, labeledTokenID+"\tmanager laptop\trevoked") {
		t.Fatalf("token list after revoke = %q, want labeled token revoked", listOutput)
	}

	registryPath := filepath.Join(dataDir, registryFile)
	projects, _, err := loadProjectRegistry(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 || projects[0].Name != "Ops" {
		t.Fatalf("cloud projects = %#v, want Ops", projects)
	}
	initialActive, err := projectAccessTokenActive(registryPath, projects[0].ID, hashToken(initialToken))
	if err != nil {
		t.Fatal(err)
	}
	if !initialActive {
		t.Fatal("initial project token should still be active")
	}
	labeledActive, err := projectAccessTokenActive(registryPath, projects[0].ID, hashToken(labeledToken))
	if err != nil {
		t.Fatal(err)
	}
	if labeledActive {
		t.Fatal("revoked project token should not be active")
	}
}

func TestCloudBackupRestoreCLICommands(t *testing.T) {
	dir := t.TempDir()
	bin := buildPromagTestBinary(t, dir)
	dataDir := filepath.Join(dir, "cloud-data")

	createOutput := runPromagCLI(t, bin, "--cloud-create", "--data-dir", dataDir, "Ops")
	if !strings.Contains(createOutput, "Created cloud project \"Ops\"") {
		t.Fatalf("cloud create output = %q", createOutput)
	}
	projectToken := extractOutputValue(t, createOutput, "Project token: ")

	registryPath := filepath.Join(dataDir, registryFile)
	projects, _, err := loadProjectRegistry(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 {
		t.Fatalf("cloud projects = %#v, want one project", projects)
	}
	project := projects[0]
	if _, err := createTask(project.DBPath, task{Title: "CLI backed up task", Priority: "medium", Status: "open"}, "manager-1"); err != nil {
		t.Fatal(err)
	}

	backupPath := filepath.Join(dir, "cloud-backup.json")
	backupOutput := runPromagCLI(t, bin, "--cloud-backup", backupPath, "--data-dir", dataDir)
	if !strings.Contains(backupOutput, "Backed up 1 cloud project(s) to "+backupPath) {
		t.Fatalf("cloud backup output = %q", backupOutput)
	}
	backupData, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(backupData, []byte(projectToken)) {
		t.Fatal("cloud backup CLI wrote raw project token")
	}

	restoreDataDir := filepath.Join(dir, "cloud-data-restored")
	restoreOutput := runPromagCLI(t, bin, "--cloud-restore", backupPath, "--data-dir", restoreDataDir)
	if !strings.Contains(restoreOutput, "Restored 1 cloud project(s) from "+backupPath) {
		t.Fatalf("cloud restore output = %q", restoreOutput)
	}
	restoredRegistryPath := filepath.Join(restoreDataDir, registryFile)
	restoredProjects, _, err := loadProjectRegistry(restoredRegistryPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(restoredProjects) != 1 || restoredProjects[0].ID != project.ID {
		t.Fatalf("restored projects = %#v, want project ID %s", restoredProjects, project.ID)
	}
	restoredState, err := loadState(restoredProjects[0].DBPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(restoredState.Tasks) != 1 || restoredState.Tasks[0].Title != "CLI backed up task" {
		t.Fatalf("restored state = %#v", restoredState)
	}
	active, err := projectAccessTokenActive(restoredRegistryPath, restoredProjects[0].ID, hashToken(projectToken))
	if err != nil {
		t.Fatal(err)
	}
	if !active {
		t.Fatal("restored project token should remain active")
	}

	listOutput := runPromagCLI(t, bin, "--cloud-tokens", "Ops", "--data-dir", restoreDataDir)
	if !strings.Contains(listOutput, "\tdefault\tactive") {
		t.Fatalf("restored cloud tokens output = %q, want default active token", listOutput)
	}
}

func TestHTTPAccessLogging(t *testing.T) {
	var log bytes.Buffer
	previous := accessLogWriter
	accessLogWriter = &log
	defer func() {
		accessLogWriter = previous
	}()

	dir := t.TempDir()
	registryPath := filepath.Join(dir, ".promag-cloud", registryFile)
	projectsBaseDir := filepath.Join(dir, ".promag-cloud", projectsDir)
	handler := newCloudServer(registryPath, projectsBaseDir, "secret")

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("health status = %d", resp.Code)
	}
	got := log.String()
	for _, want := range []string{"access", "scope=cloud", "method=GET", "path=/health", "status=200"} {
		if !strings.Contains(got, want) {
			t.Fatalf("access log = %q, want %q", got, want)
		}
	}
}

func TestCloudBackupRestorePreservesProjectsAndTokens(t *testing.T) {
	dir := t.TempDir()
	registryPath := filepath.Join(dir, ".promag-cloud", registryFile)
	projectsBaseDir := filepath.Join(dir, ".promag-cloud", projectsDir)
	project, err := createCloudProject(registryPath, projectsBaseDir, nil, "Cloud Backup")
	if err != nil {
		t.Fatal(err)
	}
	project, projectToken, err := rotateProjectAccessToken(registryPath, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := createTask(project.DBPath, task{Title: "Backed up task", Priority: "medium", Status: "open"}, "manager-1"); err != nil {
		t.Fatal(err)
	}
	_, namedToken, rawNamedToken, err := createProjectAccessToken(registryPath, project.ID, "Manager One", "admin-1")
	if err != nil {
		t.Fatal(err)
	}

	backupPath := filepath.Join(dir, "cloud-backup.json")
	if err := backupCloudProjects(registryPath, backupPath); err != nil {
		t.Fatal(err)
	}
	backupData, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(backupData, []byte(projectToken)) {
		t.Fatal("cloud backup contained raw project token")
	}
	if bytes.Contains(backupData, []byte(rawNamedToken)) {
		t.Fatal("cloud backup contained raw named token")
	}

	restoreRegistryPath := filepath.Join(dir, ".promag-cloud-restored", registryFile)
	restoreProjectsBaseDir := filepath.Join(dir, ".promag-cloud-restored", projectsDir)
	restoredCount, err := restoreCloudProjects(restoreRegistryPath, restoreProjectsBaseDir, backupPath)
	if err != nil {
		t.Fatal(err)
	}
	if restoredCount != 1 {
		t.Fatalf("restored count = %d, want 1", restoredCount)
	}
	restoredProjects, _, err := loadProjectRegistry(restoreRegistryPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(restoredProjects) != 1 {
		t.Fatalf("restored projects = %#v", restoredProjects)
	}
	restored := restoredProjects[0]
	if restored.ID != project.ID {
		t.Fatalf("restored project ID = %q, want %q", restored.ID, project.ID)
	}
	if restored.AccessTokenHash != project.AccessTokenHash {
		t.Fatalf("restored token hash = %q, want %q", restored.AccessTokenHash, project.AccessTokenHash)
	}
	restoredTokens, err := loadProjectAccessTokens(restoreRegistryPath, restored.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(restoredTokens) != 1 || restoredTokens[0].ID != namedToken.ID {
		t.Fatalf("restored tokens = %#v, want named token %s", restoredTokens, namedToken.ID)
	}
	state, err := loadState(restored.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Tasks) != 1 || state.Tasks[0].Title != "Backed up task" {
		t.Fatalf("restored state = %#v", state)
	}

	handler := newCloudServer(restoreRegistryPath, restoreProjectsBaseDir, "admin-secret")
	req := httptest.NewRequest(http.MethodGet, "/projects/"+restored.ID+"/state", nil)
	req.Header.Set("Authorization", "Bearer "+projectToken)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("restored project token status = %d body = %s", resp.Code, resp.Body.String())
	}
	req = httptest.NewRequest(http.MethodGet, "/projects/"+restored.ID+"/state", nil)
	req.Header.Set("Authorization", "Bearer "+rawNamedToken)
	resp = httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("restored named token status = %d body = %s", resp.Code, resp.Body.String())
	}
}

func TestCloudRestoreRequiresEmptyRegistry(t *testing.T) {
	dir := t.TempDir()
	sourceRegistryPath := filepath.Join(dir, "source", registryFile)
	sourceProjectsBaseDir := filepath.Join(dir, "source", projectsDir)
	if _, err := createCloudProject(sourceRegistryPath, sourceProjectsBaseDir, nil, "Source"); err != nil {
		t.Fatal(err)
	}
	backupPath := filepath.Join(dir, "cloud-backup.json")
	if err := backupCloudProjects(sourceRegistryPath, backupPath); err != nil {
		t.Fatal(err)
	}

	targetRegistryPath := filepath.Join(dir, "target", registryFile)
	targetProjectsBaseDir := filepath.Join(dir, "target", projectsDir)
	if _, err := createCloudProject(targetRegistryPath, targetProjectsBaseDir, nil, "Existing"); err != nil {
		t.Fatal(err)
	}
	if _, err := restoreCloudProjects(targetRegistryPath, targetProjectsBaseDir, backupPath); err == nil {
		t.Fatal("expected restore into non-empty registry to fail")
	}
}

func TestProjectRegistryAddsAccessTokenHashColumn(t *testing.T) {
	dir := t.TempDir()
	registryPath := filepath.Join(dir, storageDir, registryFile)
	if err := os.MkdirAll(filepath.Dir(registryPath), 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", registryPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		CREATE TABLE projects (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			type TEXT NOT NULL,
			remote_url TEXT NOT NULL,
			db_path TEXT NOT NULL,
			created_at TEXT NOT NULL,
			last_opened_at TEXT NOT NULL
		);
		CREATE TABLE app_meta (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);
		INSERT INTO projects (id, name, type, remote_url, db_path, created_at, last_opened_at)
		VALUES ('prj-old', 'Old Registry', 'local', '', 'old.sqlite3', '2026-05-18T12:00:00Z', '2026-05-18T12:00:00Z');
	`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	projects, _, err := loadProjectRegistry(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 || projects[0].AccessTokenHash != "" {
		t.Fatalf("projects after registry migration = %#v", projects)
	}
	rotated, token, err := rotateProjectAccessToken(registryPath, "prj-old")
	if err != nil {
		t.Fatal(err)
	}
	if token == "" || rotated.AccessTokenHash == "" || rotated.AccessTokenHash == token {
		t.Fatalf("rotated token/hash = token %q hash %q", token, rotated.AccessTokenHash)
	}
}

func TestProjectServerTouchesCollaboratorOnRead(t *testing.T) {
	dir := t.TempDir()
	registryPath := filepath.Join(dir, storageDir, registryFile)
	projectsBaseDir := filepath.Join(dir, storageDir, projectsDir)
	project, err := createProjectRecord(registryPath, projectsBaseDir, "Presence", projectTypeLocal, "", "")
	if err != nil {
		t.Fatal(err)
	}
	handler := newProjectServer(project, "secret")

	resp := serveJSONRequest(t, handler, http.MethodGet, "/state", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("state status = %d body = %s", resp.Code, resp.Body.String())
	}
	collaborators, err := loadCollaborators(project.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	if !hasCollaborator(collaborators, "manager-1") {
		t.Fatalf("collaborators = %#v, want manager-1 from read request", collaborators)
	}
}

func TestHandleRemoteRefreshAppliesLatestState(t *testing.T) {
	project := projectRecord{ID: "remote-1", Name: "Remote", Type: projectTypeRemote}
	m := newModel("", "", project, []projectRecord{project}, appState{}, defaultConfig())
	msg := remoteRefreshMsg{
		ProjectID: project.ID,
		Response: stateResponse{
			Config: appConfig{LeftWheelMode: "scroll_list", TaskSortMode: string(taskSortTitle)},
			State: appState{
				Tasks: []task{{ID: "tsk-1", Title: "Refreshed", Status: "open", Priority: "medium", Version: 1}},
			},
			Collaborators: []collaborator{{ID: "manager-1", Name: "Manager One", LastSeenAt: time.Now()}},
			ActivityLog:   []activityEvent{{ID: "act-1", ActorID: "manager-1", Summary: "updated task", CreatedAt: time.Now()}},
		},
	}

	updated, _ := m.handleRemoteRefresh(msg)
	got := updated.(model)
	if len(got.state.Tasks) != 1 || got.state.Tasks[0].Title != "Refreshed" {
		t.Fatalf("refreshed tasks = %#v", got.state.Tasks)
	}
	if got.config.taskSortMode() != taskSortTitle {
		t.Fatalf("sort mode = %q, want title", got.config.taskSortMode())
	}
	if len(got.collaborators) != 1 || got.collaborators[0].ID != "manager-1" {
		t.Fatalf("collaborators = %#v", got.collaborators)
	}
	if len(got.activityLog) != 1 || got.activityLog[0].ID != "act-1" {
		t.Fatalf("activity log = %#v", got.activityLog)
	}
	if got.remoteStatus != remoteStatusConnected {
		t.Fatalf("remote status = %q, want connected", got.remoteStatus)
	}
	if got.remoteLastSync.IsZero() {
		t.Fatal("remote last sync was not set")
	}
	if got.remoteLastError != "" {
		t.Fatalf("remote last error = %q, want empty", got.remoteLastError)
	}
}

func TestHandleRemoteRefreshTracksFailure(t *testing.T) {
	project := projectRecord{ID: "remote-1", Name: "Remote", Type: projectTypeRemote}
	m := newModel("", "", project, []projectRecord{project}, appState{}, defaultConfig())
	m.markRemoteSynced(time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	updated, _ := m.handleRemoteRefresh(remoteRefreshMsg{ProjectID: project.ID, Err: errors.New("server down")})
	got := updated.(model)
	if got.remoteStatus != remoteStatusFailed {
		t.Fatalf("remote status = %q, want failed", got.remoteStatus)
	}
	if got.remoteLastError != "server down" {
		t.Fatalf("remote last error = %q, want server down", got.remoteLastError)
	}
	if got.remoteLastSync.IsZero() {
		t.Fatal("remote last sync should keep the previous successful sync time")
	}
}

func TestRemoteStatusSummary(t *testing.T) {
	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	m := model{
		currentProject: projectRecord{Type: projectTypeRemote},
		remoteStatus:   remoteStatusConnected,
		remoteLastSync: now.Add(-2 * time.Minute),
	}
	if got := m.remoteStatusSummary(now); !strings.Contains(got, "Remote connected") || !strings.Contains(got, "2m ago") {
		t.Fatalf("remote status summary = %q", got)
	}

	m.remoteStatus = remoteStatusFailed
	m.remoteLastError = "timeout"
	if got := m.remoteStatusSummary(now); !strings.Contains(got, "Remote failed") || !strings.Contains(got, "timeout") {
		t.Fatalf("remote status summary = %q", got)
	}
}

func TestConflictMessagesIncludeLatestEditorContext(t *testing.T) {
	now := time.Now()
	m := model{
		collaborators: []collaborator{{ID: "manager-1", Name: "Manager One"}},
		state: appState{
			Tasks:   []task{{ID: "tsk-1", Title: "Roadmap", UpdatedBy: "manager-1", UpdatedAt: now.Add(-2 * time.Minute)}},
			Members: []member{{ID: "mem-1", Name: "Sara", UpdatedBy: "manager-1", UpdatedAt: now.Add(-3 * time.Minute)}},
		},
	}

	taskMessage := m.taskConflictMessage("tsk-1")
	for _, want := range []string{"Task \"Roadmap\"", "Manager One", "Reloaded latest version"} {
		if !strings.Contains(taskMessage, want) {
			t.Fatalf("task conflict message = %q, want to contain %q", taskMessage, want)
		}
	}

	memberMessage := m.memberConflictMessage("mem-1")
	for _, want := range []string{"Member \"Sara\"", "Manager One", "Reloaded latest version"} {
		if !strings.Contains(memberMessage, want) {
			t.Fatalf("member conflict message = %q, want to contain %q", memberMessage, want)
		}
	}
}

func TestConflictMessageFallsBackWithoutMetadata(t *testing.T) {
	m := model{state: appState{Tasks: []task{{ID: "tsk-1", Title: "Roadmap"}}}}
	if got := m.taskConflictMessage("tsk-1"); got != taskConflictFallback {
		t.Fatalf("task conflict message = %q, want fallback", got)
	}
	if got := m.taskConflictMessage("missing"); !strings.Contains(got, "removed elsewhere") {
		t.Fatalf("missing task conflict message = %q", got)
	}
}

func TestActiveCollaboratorCountUsesLastSeenWindow(t *testing.T) {
	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	m := model{collaborators: []collaborator{
		{ID: "active", LastSeenAt: now.Add(-activeUserWindow + time.Second)},
		{ID: "stale", LastSeenAt: now.Add(-activeUserWindow - time.Second)},
		{ID: "unknown"},
	}}

	if got := m.activeCollaboratorCount(now); got != 1 {
		t.Fatalf("active collaborator count = %d, want 1", got)
	}
}

func TestCollaborationDetailShowsActiveUsersAndRecentActivity(t *testing.T) {
	now := time.Now()
	m := model{
		collaborators: []collaborator{{ID: "manager-1", Name: "Manager One", Role: "manager", LastSeenAt: now}},
		activityLog:   []activityEvent{{ID: "act-1", ActorID: "manager-1", Summary: "updated task Roadmap", CreatedAt: now.Add(-2 * time.Minute)}},
	}

	got := m.collaborationDetail()
	for _, want := range []string{"Manager One (manager)", "updated task Roadmap"} {
		if !strings.Contains(got, want) {
			t.Fatalf("collaboration detail = %q, want to contain %q", got, want)
		}
	}
}

func TestRecentActivityLinesUsesRelativeTime(t *testing.T) {
	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	m := model{
		collaborators: []collaborator{{ID: "manager-1", Name: "Manager One"}},
		activityLog:   []activityEvent{{ID: "act-1", ActorID: "manager-1", Summary: "updated task Roadmap", CreatedAt: now.Add(-2 * time.Minute)}},
	}

	lines := m.recentActivityLines(now, 5)
	if len(lines) != 1 {
		t.Fatalf("activity lines = %#v, want one line", lines)
	}
	for _, want := range []string{"updated task Roadmap", "Manager One", "2m ago"} {
		if !strings.Contains(lines[0], want) {
			t.Fatalf("activity line = %q, want to contain %q", lines[0], want)
		}
	}
}

func serveJSONRequest(t *testing.T, handler http.Handler, method, path string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	return serveJSONRequestWithAuth(t, handler, method, path, body, "secret", "manager-1")
}

func serveJSONRequestWithAuth(t *testing.T, handler http.Handler, method, path string, body []byte, token, actor string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-ProMag-Actor", actor)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	return resp
}

func buildPromagTestBinary(t *testing.T, dir string) string {
	t.Helper()
	bin := filepath.Join(dir, "promag-test")
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = "."
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build promag test binary: %v\n%s", err, output)
	}
	return bin
}

func runPromagCLI(t *testing.T, bin string, args ...string) string {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = t.TempDir()
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("promag %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}
	return string(output)
}

func extractOutputValue(t *testing.T, output, prefix string) string {
	t.Helper()
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, prefix) {
			value := strings.TrimSpace(strings.TrimPrefix(line, prefix))
			if value == "" {
				t.Fatalf("empty output value for %q in %q", prefix, output)
			}
			return value
		}
	}
	t.Fatalf("missing output prefix %q in %q", prefix, output)
	return ""
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

func hasActivityAction(events []activityEvent, action string) bool {
	for _, event := range events {
		if event.Action == action {
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
