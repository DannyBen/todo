package store

import (
	"errors"
	"reflect"
	"strings"
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

func TestDuplicateAddsLeaveStoreUnchanged(t *testing.T) {
	for _, test := range []struct {
		name, wantError string
		descriptions    []string
	}{
		{name: "single", wantError: "task already exists (1)"},
		{name: "stored duplicate", descriptions: []string{"New", "Existing"}, wantError: "task already exists (1)"},
		{name: "input duplicate", descriptions: []string{"New", "New"}, wantError: "duplicate task description \"New\" in input"},
	} {
		t.Run(test.name, func(t *testing.T) {
			database, dir := openBackupStore(t)
			if _, err := database.Add("Existing", []string{"done"}, nil); err != nil {
				t.Fatal(err)
			}
			before, err := database.List()
			if err != nil {
				t.Fatal(err)
			}
			filesBefore := backupFiles(t, dir)
			if test.descriptions == nil {
				_, err = database.Add("Existing", []string{"ready"}, nil)
			} else {
				var changes []task.Change
				for _, description := range test.descriptions {
					changes = append(changes, task.Change{Description: &description})
				}
				var created []task.Task
				created, err = database.AddMany(changes)
				var itemErr *AddManyError
				if len(created) != 0 || !errors.As(err, &itemErr) || itemErr.Index != 1 {
					t.Fatalf("tasks = %v, error = %v; want no tasks and failure on second item", created, err)
				}
			}
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("error = %v, want %q", err, test.wantError)
			}
			after, err := database.List()
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatalf("tasks = %v, error = %v; want %v", after, err, before)
			}
			if files := backupFiles(t, dir); !reflect.DeepEqual(filesBefore, files) {
				t.Fatalf("duplicate left snapshots: %v, want %v", files, filesBefore)
			}
			created, err := database.Add("New", nil, nil)
			if err != nil || created.ID != 2 {
				t.Fatalf("next add = %v, error = %v; want task 2", created, err)
			}
		})
	}
}
