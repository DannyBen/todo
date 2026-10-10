package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommandsWithoutDatabase(t *testing.T) {
	for _, test := range []struct {
		args    []string
		wantErr string
	}{
		{args: []string{"list"}},
		{args: []string{"l", "+now"}},
		{args: []string{"ls", "1..10"}},
		{args: []string{"list", "=1"}, wantErr: "filter prefix = is reserved"},
		{args: []string{"edit", "1", "+done"}, wantErr: "no todo database exists"},
		{args: []string{"e", "1"}, wantErr: "no todo database exists"},
		{args: []string{"del", "+done"}, wantErr: "no todo database exists"},
		{args: []string{"d", "1"}, wantErr: "no todo database exists"},
		{args: []string{"r", "1"}, wantErr: "no todo database exists"},
		{args: []string{"rm", "1"}, wantErr: "no todo database exists"},
	} {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("TODO_FILE", filepath.Join(dir, "tasks.sqlite"))
			t.Setenv("TODO_BACKUP", "on")
			var stdout, stderr bytes.Buffer
			err := ExecuteWithIO(test.args, "test", strings.NewReader(""), &stdout, &stderr)
			if test.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, test.wantErr)
				}
				PrintError(err, &stderr)
				if !strings.Contains(stderr.String(), test.wantErr) {
					t.Fatalf("stderr = %q, want containing %q", stderr.String(), test.wantErr)
				}
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want no output", stdout.String())
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 0 {
				t.Fatalf("command created files: %v", entries)
			}
		})
	}
}

func TestListWithoutDatabaseAtProjectLocation(t *testing.T) {
	for _, gitRepo := range []bool{false, true} {
		name := "outside Git"
		if gitRepo {
			name = "nested Git directory"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			workingDir := root
			if gitRepo {
				if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
					t.Fatal(err)
				}
				workingDir = filepath.Join(root, "nested")
				if err := os.Mkdir(workingDir, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			t.Chdir(workingDir)
			t.Setenv("TODO_FILE", "")
			t.Setenv("TODO_BACKUP", "on")
			var stdout bytes.Buffer
			if err := Execute([]string{"ls"}, "test", &stdout); err != nil {
				t.Fatal(err)
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want no output", stdout.String())
			}
			for _, dir := range []string{root, workingDir} {
				entries, err := os.ReadDir(dir)
				if err != nil {
					t.Fatal(err)
				}
				for _, entry := range entries {
					if entry.Name() != ".git" && entry.Name() != "nested" {
						t.Fatalf("command created %q in %q", entry.Name(), dir)
					}
				}
			}
		})
	}
}

func TestListRejectsInvalidExistingDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.sqlite")
	t.Setenv("TODO_FILE", path)
	content := []byte("not a SQLite database")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	if err := Execute([]string{"ls"}, "test", &stdout); err == nil {
		t.Fatal("invalid existing database must return an error")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want no output", stdout.String())
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("invalid database changed: %q", got)
	}
}
