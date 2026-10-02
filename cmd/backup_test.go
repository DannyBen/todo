package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestCLIBackupsUseDatabaseDirectory(t *testing.T) {
	root := t.TempDir()
	t.Setenv("TODO_FILE", filepath.Join(root, ".todo.sqlite"))
	t.Setenv("TODO_BACKUP", "2@.todo")
	for _, args := range [][]string{
		{"add", "First"}, {"add", "Second"},
		{"edit", "1..", "+ready"}, {"del", "1.."},
	} {
		if err := Execute(args, "test", &bytes.Buffer{}); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(filepath.Join(root, ".todo"))
	if err != nil || len(entries) != 3 {
		t.Fatalf("backup directory entries=%v err=%v", entries, err)
	}
	var stdout bytes.Buffer
	if err := Execute([]string{"ls"}, "test", &stdout); err != nil || stdout.Len() != 0 {
		t.Fatalf("delete did not empty tasks: stdout=%q err=%v", stdout.String(), err)
	}
	// Reading works even when the configured backup directory is unavailable.
	t.Setenv("TODO_BACKUP", filepath.Join(root, ".todo.sqlite"))
	if err := Execute([]string{"ls"}, "test", &stdout); err != nil {
		t.Fatal(err)
	}
	if err := Execute([]string{"add", "Blocked"}, "test", &stdout); err == nil {
		t.Fatal("mutation succeeded without a usable backup directory")
	}
	if err := Execute([]string{"ls"}, "test", &stdout); err != nil || stdout.Len() != 0 {
		t.Fatalf("backup failure changed tasks: stdout=%q err=%v", stdout.String(), err)
	}
}
