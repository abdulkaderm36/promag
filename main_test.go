package main

import (
	"path/filepath"
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
