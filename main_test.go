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

func TestRemoteProjectClientLoadsAndMutatesThroughServer(t *testing.T) {
	dir := t.TempDir()
	registryPath := filepath.Join(dir, storageDir, registryFile)
	projectsBaseDir := filepath.Join(dir, storageDir, projectsDir)
	serverProject, err := createProjectRecord(registryPath, projectsBaseDir, "Server", projectTypeLocal, "")
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
	project, err := createProjectRecord(registryPath, projectsBaseDir, "Presence", projectTypeLocal, "")
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
