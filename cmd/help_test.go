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
	var stdout bytes.Buffer
	if err := Execute([]string{"--help"}, "1.2.3", &stdout); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"todo add TEXT...",
		"todo list FILTER...",
		"todo edit ID TEXT...",
		"todo del ID",
		"12       task ID is 12",
	} {
		if !strings.Contains(stdout.String(), expected) {
			t.Fatalf("help does not contain %q:\n%s", expected, stdout.String())
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
	if err := Execute([]string{"add", "Prepare", "deployment", "+now"}, "test", &stdout); err != nil {
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

	if err := Execute([]string{"del", "1"}, "test", &stdout); err != nil {
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
