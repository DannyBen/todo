package store

import (
	"errors"
	"testing"

	"github.com/dannyben/todo/internal/task"
)

func TestAddManyRollbackAndBackup(t *testing.T) {
	database, dir := openBackupStore(t)
	first, second := "First", "Second"
	changes := []task.Change{{Description: &first}, {Description: &second}}
	if _, err := database.db.Exec(`CREATE TRIGGER fail_second BEFORE INSERT ON tasks
		WHEN NEW.description = 'Second' BEGIN SELECT RAISE(ABORT, 'insertion failed'); END`); err != nil {
		t.Fatal(err)
	}
	_, err := database.AddMany(changes)
	var itemErr *AddManyError
	if !errors.As(err, &itemErr) || itemErr.Index != 1 {
		t.Fatalf("error = %v, want failure on second item", err)
	}
	items, err := database.List()
	if err != nil || len(items) != 0 {
		t.Fatalf("tasks = %v, err = %v", items, err)
	}
	if files := backupFiles(t, dir); len(files) != 0 {
		t.Fatalf("failed batch left snapshots: %v", files)
	}
	if _, err := database.db.Exec("DROP TRIGGER fail_second"); err != nil {
		t.Fatal(err)
	}
	items, err = database.AddMany(changes)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].ID != 1 || items[1].ID != 2 {
		t.Fatalf("retry tasks = %v", items)
	}
	if files := backupFiles(t, dir); len(files) != 1 {
		t.Fatalf("batch snapshots = %v, want one", files)
	}
}
