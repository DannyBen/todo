package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHelpLocalGuidelines(t *testing.T) {
	for _, test := range []struct {
		name    string
		content string
		missing bool
		bold    bool
		want    string
	}{
		{name: "missing", missing: true},
		{name: "empty"},
		{name: "whitespace", content: " \n\t\n"},
		{
			name:    "plain text",
			content: "Mark tasks +done when done.\n# literal heading\n**literal bold**\n\n  Keep existing indentation.\n",
			want:    "\nLocal guidelines:\n\n  Mark tasks +done when done.\n  # literal heading\n  **literal bold**\n  \n    Keep existing indentation.\n",
		},
		{
			name:    "no final newline",
			content: "Write short tasks, first letter lowercase.",
			want:    "\nLocal guidelines:\n\n  Write short tasks, first letter lowercase.\n",
		},
		{
			name:    "bold caption only",
			content: "**plain text**\n",
			bold:    true,
			want:    "\n\x1b[1mLocal guidelines:\x1b[0m\n\n  **plain text**\n",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("TODO_FILE", filepath.Join(dir, "tasks.sqlite"))
			if !test.missing {
				if err := os.WriteFile(filepath.Join(dir, guidelinesFilename), []byte(test.content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			var stdout bytes.Buffer
			if err := runHelp(&stdout, test.bold); err != nil {
				t.Fatal(err)
			}
			if got, want := stdout.String(), formatManual(test.bold)+test.want; got != want {
				t.Fatalf("help = %q, want %q", got, want)
			}
			if _, err := os.Stat(filepath.Join(dir, "tasks.sqlite")); !os.IsNotExist(err) {
				t.Fatalf("help created database: %v", err)
			}
		})
	}
}

func TestHelpGuidelinesLocation(t *testing.T) {
	for _, location := range []string{"current directory", "Git root", "TODO_FILE", "relative TODO_FILE"} {
		t.Run(location, func(t *testing.T) {
			root := t.TempDir()
			workingDir, guidelinesDir := root, root
			t.Setenv("TODO_FILE", "")
			switch location {
			case "Git root":
				if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
					t.Fatal(err)
				}
				workingDir = filepath.Join(root, "nested")
				if err := os.Mkdir(workingDir, 0o755); err != nil {
					t.Fatal(err)
				}
			case "TODO_FILE", "relative TODO_FILE":
				guidelinesDir = filepath.Join(root, "data")
				if err := os.Mkdir(guidelinesDir, 0o755); err != nil {
					t.Fatal(err)
				}
				dbFile := filepath.Join(guidelinesDir, "custom.sqlite")
				if location == "relative TODO_FILE" {
					dbFile = filepath.Join("data", "custom.sqlite")
				}
				t.Setenv("TODO_FILE", dbFile)
			}
			t.Chdir(workingDir)
			if err := os.WriteFile(filepath.Join(guidelinesDir, guidelinesFilename), []byte("Mark tasks +done."), 0o600); err != nil {
				t.Fatal(err)
			}
			if workingDir != guidelinesDir {
				if err := os.WriteFile(filepath.Join(workingDir, guidelinesFilename), []byte("Wrong location."), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			for _, command := range []string{"help", "h", "--help", "-h"} {
				var stdout bytes.Buffer
				if err := Execute([]string{command}, "test", &stdout); err != nil {
					t.Fatal(err)
				}
				if !strings.HasSuffix(stdout.String(), "\nLocal guidelines:\n\n  Mark tasks +done.\n") || strings.Contains(stdout.String(), "Wrong location.") {
					t.Fatalf("%s loaded wrong guidelines:\n%s", command, stdout.String())
				}
			}
		})
	}
}

func TestHelpGuidelinesReadError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TODO_FILE", filepath.Join(dir, "tasks.sqlite"))
	if err := os.Mkdir(filepath.Join(dir, guidelinesFilename), 0o755); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	err := Execute([]string{"help"}, "test", &stdout)
	if err == nil || !strings.Contains(err.Error(), "read local guidelines:") {
		t.Fatalf("error = %v, want guidelines read error", err)
	}
}
