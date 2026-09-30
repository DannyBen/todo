package cmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHelp(t *testing.T) {
	for _, command := range []string{"help", "--help", "-h"} {
		t.Run(command, func(t *testing.T) {
			var stdout bytes.Buffer
			if err := Execute([]string{command}, "1.2.3", &stdout); err != nil {
				t.Fatal(err)
			}
			for _, expected := range []string{
				"todo add TEXT...",
				"todo help",
				"todo new = todo add",
				"Task syntax:",
				"+TAG     add a tag",
				"-TAG     remove a tag",
				"Filters are combined with AND",
				"TODO_DB_FILE",
			} {
				if !strings.Contains(stdout.String(), expected) {
					t.Fatalf("help does not contain %q:\n%s", expected, stdout.String())
				}
			}
			if strings.Contains(stdout.String(), "**") || strings.Contains(stdout.String(), "\x1b[") {
				t.Fatalf("redirected help contains formatting markup: %q", stdout.String())
			}
		})
	}
}

func TestManualBoldFormatting(t *testing.T) {
	formatted := formatManual(true)
	for _, expected := range []string{"\x1b[1mUsage:\x1b[0m\n\n", "\x1b[1mAliases:\x1b[0m\n\n"} {
		if !strings.Contains(formatted, expected) {
			t.Fatalf("formatted manual does not contain %q:\n%s", expected, formatted)
		}
	}
	if strings.Contains(formatted, "**") {
		t.Fatalf("formatted manual contains source markers:\n%s", formatted)
	}
}

func TestMainUsageIsConcise(t *testing.T) {
	err := Execute(nil, "1.2.3", &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected usage error")
	}
	for _, expected := range []string{"todo add TEXT...", "todo help"} {
		if !strings.Contains(err.Error(), expected) {
			t.Fatalf("usage does not contain %q:\n%s", expected, err)
		}
	}
	for _, unwanted := range []string{"Task syntax:", "Filters are combined", "+TAG", "Aliases:", "Run 'todo help'"} {
		if strings.Contains(err.Error(), unwanted) {
			t.Fatalf("usage unexpectedly contains %q:\n%s", unwanted, err)
		}
	}
}

func TestPrintError(t *testing.T) {
	var stderr bytes.Buffer
	PrintError(errors.New("broken"), &stderr)
	PrintError(nil, &stderr)
	if got, want := stderr.String(), "error: broken\n"; got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}

	stderr.Reset()
	PrintError(usageError{message: "usage: todo"}, &stderr)
	if got, want := stderr.String(), "usage: todo\n"; got != want {
		t.Fatalf("usage stderr = %q, want %q", got, want)
	}
}

func TestVersion(t *testing.T) {
	var stdout bytes.Buffer
	if err := Execute([]string{"--version"}, "1.2.3", &stdout); err != nil {
		t.Fatal(err)
	}
	if got, want := stdout.String(), "1.2.3\n"; got != want {
		t.Fatalf("version output = %q, want %q", got, want)
	}
}

func TestUsageErrors(t *testing.T) {
	t.Setenv("TODO_DB_FILE", filepath.Join(t.TempDir(), "tasks.sqlite"))
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "no command", want: "Todo - A lightweight project todo list"},
		{name: "unknown command", args: []string{"nope"}, want: `unknown command "nope"`},
		{name: "add without text", args: []string{"add"}, want: "usage: todo add TEXT..."},
		{name: "edit without text", args: []string{"edit", "1"}, want: "usage: todo edit ID TEXT..."},
		{name: "delete without ID", args: []string{"del"}, want: "usage: todo del ID"},
		{name: "invalid edit ID", args: []string{"edit", "bad", "+done"}, want: `invalid task ID "bad"`},
		{name: "invalid delete ID", args: []string{"del", "0"}, want: `invalid task ID "0"`},
		{name: "reserved filter", args: []string{"list", "=1"}, want: "filter prefix = is reserved"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout bytes.Buffer
			err := Execute(test.args, "test", &stdout)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestDatabaseFileResolution(t *testing.T) {
	configured := filepath.Join(t.TempDir(), "configured.sqlite")
	t.Setenv("TODO_DB_FILE", configured)
	if got, err := databaseFile(); err != nil || got != configured {
		t.Fatalf("databaseFile() = %q, %v; want %q", got, err, configured)
	}

	t.Setenv("TODO_DB_FILE", "")
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "one", "two")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	oldWorkingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(nested); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWorkingDirectory); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})

	if got, err := databaseFile(); err != nil || got != filepath.Join(root, ".todo.sqlite") {
		t.Fatalf("databaseFile() = %q, %v; want Git-root database", got, err)
	}
}

func TestCLIWorkflow(t *testing.T) {
	t.Setenv("TODO_DB_FILE", filepath.Join(t.TempDir(), "tasks.sqlite"))

	var stdout bytes.Buffer
	if err := Execute([]string{"new", "Prepare", "deployment", "+now"}, "test", &stdout); err != nil {
		t.Fatal(err)
	}
	if got, want := stdout.String(), "1 Prepare deployment +now\n"; got != want {
		t.Fatalf("add output = %q, want %q", got, want)
	}

	stdout.Reset()
	if err := Execute([]string{"add", "Review", "deployment", "-", "carefully", "+ready", "@1"}, "test", &stdout); err != nil {
		t.Fatal(err)
	}
	if got, want := stdout.String(), "2 Review deployment - carefully +ready @1\n"; got != want {
		t.Fatalf("second add output = %q, want %q", got, want)
	}

	stdout.Reset()
	filters := []string{"ls", "deployment", "+ready", "-blocked", "@1", "2"}
	if err := Execute(filters, "test", &stdout); err != nil {
		t.Fatal(err)
	}
	if got, want := stdout.String(), "2 Review deployment - carefully +ready @1\n"; got != want {
		t.Fatalf("filtered list output = %q, want %q", got, want)
	}

	stdout.Reset()
	if err := Execute([]string{"edit", "2", "-ready", "+done", "-@1"}, "test", &stdout); err != nil {
		t.Fatal(err)
	}
	if got, want := stdout.String(), "2 Review deployment - carefully +done\n"; got != want {
		t.Fatalf("edit output = %q, want %q", got, want)
	}

	if err := Execute([]string{"rm", "1"}, "test", &stdout); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	if err := Execute([]string{"list"}, "test", &stdout); err != nil {
		t.Fatal(err)
	}
	if got, want := stdout.String(), "2 Review deployment - carefully +done\n"; got != want {
		t.Fatalf("list after deletion = %q, want %q", got, want)
	}

	stdout.Reset()
	if err := Execute([]string{"add", "Hello", "-now"}, "test", &stdout); err != nil {
		t.Fatal(err)
	}
	if got, want := stdout.String(), "3 Hello\n"; got != want {
		t.Fatalf("add with removal output = %q, want %q", got, want)
	}
}
