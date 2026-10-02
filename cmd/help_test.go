package cmd

import (
	"bytes"
	"errors"
	"io"
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
				"todo del FILTER...",
				"todo help",
				"todo new = todo add",
				"Arguments:",
				"Operators:",
				"Everything is ordinary text except the operator forms below.",
				"+  Add or Has",
				"/  OR when selecting tasks",
				"todo rm 3/4/5/+low",
				".. Inclusive task ID range",
				"todo edit 2..4/8 +ready",
				"Ranges select existing IDs, not a count of tasks.",
				"Bulk selectors (any range or selections joined with `/`) cannot replace task",
				":  Treat a leading + or - as text",
				"todo add Test the :-h flag and :+draft syntax",
				"todo list :-h",
				"Spaces between filters mean AND",
				"Examples:",
				"todo edit 1 -now +done",
				"List task 2 OR tasks connected to task 2:",
				"todo list 2/+2",
				"todo del +done",
				"TODO_FILE",
				"Backups:",
				"export TODO_BACKUP=on",
				"export TODO_BACKUP=.todo",
				"export TODO_BACKUP=30@.todo",
				".backupid",
				"NO_COLOR",
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
	for _, expected := range []string{"todo add TEXT...", "todo edit ID [TEXT...]", "todo del FILTER...", "todo help"} {
		if !strings.Contains(err.Error(), expected) {
			t.Fatalf("usage does not contain %q:\n%s", expected, err)
		}
	}
	for _, unwanted := range []string{"Task syntax:", "Filters are combined", "Aliases:", "Run 'todo help'"} {
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
	t.Setenv("TODO_FILE", filepath.Join(t.TempDir(), "tasks.sqlite"))
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "no command", want: "Todo - A lightweight project todo list"},
		{name: "unknown command", args: []string{"nope"}, want: `unknown command "nope"`},
		{name: "add without text", args: []string{"add"}, want: "usage: todo add TEXT..."},
		{name: "delete without filter", args: []string{"del"}, want: "usage: todo del FILTER..."},
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

func TestInteractiveEditUsesTheSameExpressionSyntax(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TODO_FILE", filepath.Join(dir, "tasks.sqlite"))

	var stdout, stderr bytes.Buffer
	if err := Execute([]string{"add", "Typo", "task", "+ready"}, "test", &stdout); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	prompt := func(id int64, current string, stdin io.Reader, stderr io.Writer) (string, error) {
		if id != 1 {
			t.Fatalf("prompt ID = %d, want 1", id)
		}
		if current != "Typo task +ready" {
			t.Fatalf("prompt text = %q, want current task expression", current)
		}
		return "Corrected description +done -ready", nil
	}
	if err := executeWithPrompt([]string{"edit", "1"}, "test", strings.NewReader(""), &stdout, &stderr, prompt); err != nil {
		t.Fatal(err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
	if got, want := stdout.String(), "1 Corrected description +done\n"; got != want {
		t.Fatalf("interactive edit output = %q, want %q", got, want)
	}
}

func TestInteractiveEditCanBeCanceled(t *testing.T) {
	t.Setenv("TODO_FILE", filepath.Join(t.TempDir(), "tasks.sqlite"))

	var stdout bytes.Buffer
	if err := Execute([]string{"add", "Keep", "this"}, "test", &stdout); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	prompt := func(int64, string, io.Reader, io.Writer) (string, error) {
		return "", errEditCanceled
	}
	if err := executeWithPrompt([]string{"edit", "1"}, "test", strings.NewReader(""), &stdout, &bytes.Buffer{}, prompt); err != nil {
		t.Fatal(err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("canceled edit output = %q, want none", stdout.String())
	}
	if err := Execute([]string{"list", "1"}, "test", &stdout); err != nil {
		t.Fatal(err)
	}
	if got, want := stdout.String(), "1 Keep this\n"; got != want {
		t.Fatalf("task after canceled edit = %q, want %q", got, want)
	}
}

func TestInteractiveEditRequiresTerminal(t *testing.T) {
	_, err := promptEdit(1, "Task", strings.NewReader(""), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "interactive edit requires a terminal") {
		t.Fatalf("error = %v, want terminal requirement", err)
	}
}

func TestEditPromptFormatting(t *testing.T) {
	hint, prompt := editPromptText(41, true)
	if got, want := hint, "\x1b[1;36mEnter\x1b[0m saves · \x1b[1;36mCtrl+C\x1b[0m cancels"; got != want {
		t.Fatalf("colored hint = %q, want %q", got, want)
	}
	if got, want := prompt, "\x1b[1;33m41\x1b[0m "; got != want {
		t.Fatalf("colored prompt = %q, want %q", got, want)
	}

	hint, prompt = editPromptText(41, false)
	if got, want := hint, "Enter saves · Ctrl+C cancels"; got != want {
		t.Fatalf("plain hint = %q, want %q", got, want)
	}
	if got, want := prompt, "41 "; got != want {
		t.Fatalf("plain prompt = %q, want %q", got, want)
	}
}

func TestDatabaseFileResolution(t *testing.T) {
	configured := filepath.Join(t.TempDir(), "configured.sqlite")
	t.Setenv("TODO_FILE", configured)
	if got, err := databaseFile(); err != nil || got != configured {
		t.Fatalf("databaseFile() = %q, %v; want %q", got, err, configured)
	}

	t.Setenv("TODO_FILE", "")
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
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
	t.Setenv("TODO_FILE", filepath.Join(t.TempDir(), "tasks.sqlite"))

	var stdout bytes.Buffer
	if err := Execute([]string{"new", "Prepare", "deployment", "+now"}, "test", &stdout); err != nil {
		t.Fatal(err)
	}
	if got, want := stdout.String(), "1 Prepare deployment +now\n"; got != want {
		t.Fatalf("add output = %q, want %q", got, want)
	}

	stdout.Reset()
	if err := Execute([]string{"add", "Review", "deployment", "-", "carefully", "+ready", "+1"}, "test", &stdout); err != nil {
		t.Fatal(err)
	}
	if got, want := stdout.String(), "2 Review deployment - carefully +ready +1\n"; got != want {
		t.Fatalf("second add output = %q, want %q", got, want)
	}

	stdout.Reset()
	filters := []string{"ls", "deployment", "+ready", "-blocked", "+1", "2"}
	if err := Execute(filters, "test", &stdout); err != nil {
		t.Fatal(err)
	}
	if got, want := stdout.String(), "2 Review deployment - carefully +ready +1\n"; got != want {
		t.Fatalf("filtered list output = %q, want %q", got, want)
	}

	stdout.Reset()
	if err := Execute([]string{"edit", "2", "-ready", "+done", "-1"}, "test", &stdout); err != nil {
		t.Fatal(err)
	}
	if got, want := stdout.String(), "2 Review deployment - carefully +done\n"; got != want {
		t.Fatalf("edit output = %q, want %q", got, want)
	}

	stdout.Reset()
	if err := Execute([]string{"rm", "1"}, "test", &stdout); err != nil {
		t.Fatal(err)
	}
	if got, want := stdout.String(), "1 Prepare deployment +now\n"; got != want {
		t.Fatalf("delete output = %q, want %q", got, want)
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

func TestDeleteByTagFilter(t *testing.T) {
	t.Setenv("TODO_FILE", filepath.Join(t.TempDir(), "tasks.sqlite"))

	for _, args := range [][]string{
		{"add", "First", "+done"},
		{"add", "Second", "+done", "+keep", "+1"},
		{"add", "Remaining", "+keep"},
	} {
		if err := Execute(args, "test", &bytes.Buffer{}); err != nil {
			t.Fatal(err)
		}
	}

	var stdout bytes.Buffer
	if err := Execute([]string{"rm", "+done"}, "test", &stdout); err != nil {
		t.Fatal(err)
	}
	if got, want := stdout.String(), "1 First +done +2\n2 Second +done +keep +1\n"; got != want {
		t.Fatalf("bulk delete output = %q, want %q", got, want)
	}

	stdout.Reset()
	if err := Execute([]string{"list"}, "test", &stdout); err != nil {
		t.Fatal(err)
	}
	if got, want := stdout.String(), "3 Remaining +keep\n"; got != want {
		t.Fatalf("list after bulk delete = %q, want %q", got, want)
	}

	stdout.Reset()
	if err := Execute([]string{"rm", "+done"}, "test", &stdout); err != nil {
		t.Fatal(err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("empty bulk delete output = %q", stdout.String())
	}
}

func TestDeleteByMixedFilters(t *testing.T) {
	t.Setenv("TODO_FILE", filepath.Join(t.TempDir(), "tasks.sqlite"))
	for _, args := range [][]string{
		{"add", "First", "+maybe"},
		{"add", "Second", "+low"},
		{"add", "Consider third", "+keep"},
		{"add", "Remaining", "+keep"},
	} {
		if err := Execute(args, "test", &bytes.Buffer{}); err != nil {
			t.Fatal(err)
		}
	}

	var stdout bytes.Buffer
	if err := Execute([]string{"rm", "1/+low"}, "test", &stdout); err != nil {
		t.Fatal(err)
	}
	if got, want := stdout.String(), "1 First +maybe\n2 Second +low\n"; got != want {
		t.Fatalf("mixed OR delete = %q, want %q", got, want)
	}

	stdout.Reset()
	if err := Execute([]string{"rm", "consider", "+keep"}, "test", &stdout); err != nil {
		t.Fatal(err)
	}
	if got, want := stdout.String(), "3 Consider third +keep\n"; got != want {
		t.Fatalf("AND delete = %q, want %q", got, want)
	}

	stdout.Reset()
	if err := Execute([]string{"list"}, "test", &stdout); err != nil {
		t.Fatal(err)
	}
	if got, want := stdout.String(), "4 Remaining +keep\n"; got != want {
		t.Fatalf("remaining tasks = %q, want %q", got, want)
	}
}

func TestSlashSeparatedIDs(t *testing.T) {
	t.Setenv("TODO_FILE", filepath.Join(t.TempDir(), "tasks.sqlite"))
	for _, description := range []string{"First", "Second", "Third"} {
		if err := Execute([]string{"add", description}, "test", &bytes.Buffer{}); err != nil {
			t.Fatal(err)
		}
	}

	var stdout bytes.Buffer
	if err := Execute([]string{"list", "3/1"}, "test", &stdout); err != nil {
		t.Fatal(err)
	}
	if got, want := stdout.String(), "1 First\n3 Third\n"; got != want {
		t.Fatalf("multi-ID list = %q, want %q", got, want)
	}

	stdout.Reset()
	if err := Execute([]string{"edit", "3/1", "+batch"}, "test", &stdout); err != nil {
		t.Fatal(err)
	}
	if got, want := stdout.String(), "3 Third +batch\n1 First +batch\n"; got != want {
		t.Fatalf("multi-ID edit = %q, want %q", got, want)
	}

	if err := Execute([]string{"edit", "1/99", "+rollback"}, "test", &bytes.Buffer{}); err == nil {
		t.Fatal("multi-ID edit with missing task did not fail")
	}
	if err := Execute([]string{"edit", "1/2"}, "test", &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "require an edit expression") {
		t.Fatalf("interactive multi-ID edit error = %v", err)
	}

	stdout.Reset()
	if err := Execute([]string{"rm", "3/1"}, "test", &stdout); err != nil {
		t.Fatal(err)
	}
	if got, want := stdout.String(), "3 Third +batch\n1 First +batch\n"; got != want {
		t.Fatalf("multi-ID delete = %q, want %q", got, want)
	}

	stdout.Reset()
	if err := Execute([]string{"list"}, "test", &stdout); err != nil {
		t.Fatal(err)
	}
	if got, want := stdout.String(), "2 Second\n"; got != want {
		t.Fatalf("list after multi-ID delete = %q, want %q", got, want)
	}
}

func TestTagFilterOR(t *testing.T) {
	t.Setenv("TODO_FILE", filepath.Join(t.TempDir(), "tasks.sqlite"))
	for _, args := range [][]string{
		{"add", "Low", "+low"},
		{"add", "Maybe", "+maybe"},
		{"add", "Other", "+other"},
		{"add", "Deferred low", "+low", "+defer"},
	} {
		if err := Execute(args, "test", &bytes.Buffer{}); err != nil {
			t.Fatal(err)
		}
	}

	var stdout bytes.Buffer
	if err := Execute([]string{"list", "+low/+maybe", "-defer"}, "test", &stdout); err != nil {
		t.Fatal(err)
	}
	if got, want := stdout.String(), "1 Low +low\n2 Maybe +maybe\n"; got != want {
		t.Fatalf("tag OR list = %q, want %q", got, want)
	}
}
