package store

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/dannyben/todo/internal/task"
)

func TestOpenRejectsInvalidDatabasePath(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if database, err := Open(filepath.Join(file, "tasks.sqlite")); err == nil {
		database.Close()
		t.Fatal("expected invalid database path error")
	}
}

func TestTaskLifecycle(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "todo.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	first, err := database.Add("First task", []string{"ready"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := database.Add("Second task", []string{"blocked"}, []int64{first.ID})
	if err != nil {
		t.Fatal(err)
	}
	first, err = database.Get(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := task.Format(first), "1 First task +ready @2"; got != want {
		t.Fatalf("connected task = %q, want %q", got, want)
	}

	description := "Updated task"
	second, err = database.Edit(second.ID, task.Change{
		Description:      &description,
		AddTags:          []string{"ready"},
		RemoveTags:       []string{"blocked"},
		RemoveReferences: []int64{first.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := task.Format(second), "2 Updated task +ready"; got != want {
		t.Fatalf("edited task = %q, want %q", got, want)
	}

	deleted, err := database.Delete(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := task.Format(deleted), "1 First task +ready"; got != want {
		t.Fatalf("deleted task = %q, want %q", got, want)
	}
	third, err := database.Add("Third task", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if third.ID <= second.ID {
		t.Fatalf("deleted ID was reused: new ID = %d", third.ID)
	}

	tasks, err := database.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 {
		t.Fatalf("task count = %d, want 2", len(tasks))
	}
	if got, want := tasks[0].ID, second.ID; got != want {
		t.Fatalf("first listed ID = %d, want %d", got, want)
	}
	if got, want := tasks[1].ID, third.ID; got != want {
		t.Fatalf("second listed ID = %d, want %d", got, want)
	}
}

func TestConnectionCanBeRemovedFromEitherTask(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "todo.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	first, err := database.Add("First", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := database.Add("Second", nil, []int64{first.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Edit(first.ID, task.Change{RemoveReferences: []int64{second.ID}}); err != nil {
		t.Fatal(err)
	}
	second, err = database.Get(second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.References) != 0 {
		t.Fatalf("references after inverse removal = %#v", second.References)
	}
}

func TestOpenMigratesDirectedReferencesToConnections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "todo.sqlite")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	statements := []string{
		"PRAGMA foreign_keys = ON",
		"CREATE TABLE tasks (id INTEGER PRIMARY KEY AUTOINCREMENT, description TEXT NOT NULL)",
		"CREATE TABLE task_references (task_id INTEGER NOT NULL REFERENCES tasks(id), reference_id INTEGER NOT NULL REFERENCES tasks(id), PRIMARY KEY (task_id, reference_id))",
		"INSERT INTO tasks (description) VALUES ('First'), ('Second')",
		"INSERT INTO task_references (task_id, reference_id) VALUES (2, 1)",
	}
	for _, statement := range statements {
		if _, err := legacy.Exec(statement); err != nil {
			legacy.Close()
			t.Fatal(err)
		}
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	database, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	for id, want := range map[int64]string{
		1: "1 First @2",
		2: "2 Second @1",
	} {
		item, err := database.Get(id)
		if err != nil {
			t.Fatal(err)
		}
		if got := task.Format(item); got != want {
			t.Fatalf("task %d = %q, want %q", id, got, want)
		}
	}
	var legacyTables int
	if err := database.db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'task_references'").Scan(&legacyTables); err != nil {
		t.Fatal(err)
	}
	if legacyTables != 0 {
		t.Fatal("legacy task_references table still exists")
	}
}

func TestDeleteRemovesBacklinks(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "todo.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	target, err := database.Add("Target", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	source, err := database.Add("Source", nil, []int64{target.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Delete(target.ID); err != nil {
		t.Fatal(err)
	}
	source, err = database.Get(source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(source.References) != 0 {
		t.Fatalf("references after target deletion = %#v", source.References)
	}
}

func TestMissingTasksAndInvalidReferences(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "todo.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	first, err := database.Add("First", nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		run  func() error
	}{
		{name: "add empty description", run: func() error {
			_, err := database.Add("", nil, nil)
			return err
		}},
		{name: "add missing reference", run: func() error {
			_, err := database.Add("Broken", nil, []int64{99})
			return err
		}},
		{name: "edit missing task", run: func() error {
			_, err := database.Edit(99, task.Change{})
			return err
		}},
		{name: "edit self reference", run: func() error {
			_, err := database.Edit(first.ID, task.Change{AddReferences: []int64{first.ID}})
			return err
		}},
		{name: "edit missing reference", run: func() error {
			_, err := database.Edit(first.ID, task.Change{AddReferences: []int64{99}})
			return err
		}},
		{name: "edit empty description", run: func() error {
			empty := ""
			_, err := database.Edit(first.ID, task.Change{Description: &empty})
			return err
		}},
		{name: "get missing task", run: func() error {
			_, err := database.Get(99)
			return err
		}},
		{name: "delete missing task", run: func() error {
			_, err := database.Delete(99)
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.run(); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
