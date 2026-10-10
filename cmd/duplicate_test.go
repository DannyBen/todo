package cmd

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestAddDuplicateDescription(t *testing.T) {
	for _, test := range []struct {
		name    string
		args    []string
		wantNew string
	}{
		{name: "exact", args: []string{"add", "Fix login"}},
		{name: "different tags", args: []string{"a", "Fix login", "+ready"}},
		{name: "different connections", args: []string{"new", "Fix login", "+2"}},
		{name: "normalized whitespace", args: []string{"n", " \tFix   login\n", "+later"}},
		{name: "different case", args: []string{"add", "fix login"}, wantNew: "3 fix login\n"},
		{name: "longer description", args: []string{"add", "Fix login again"}, wantNew: "3 Fix login again\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("TODO_FILE", filepath.Join(t.TempDir(), "tasks.sqlite"))
			t.Setenv("TODO_BACKUP", "")
			var stdout, stderr bytes.Buffer
			for _, args := range [][]string{{"add", "Fix login", "+done"}, {"add", "Other task"}} {
				if err := Execute(args, "test", &stdout); err != nil {
					t.Fatal(err)
				}
			}
			stdout.Reset()
			err := ExecuteWithIO(test.args, "test", strings.NewReader(""), &stdout, &stderr)
			if test.wantNew == "" {
				if err == nil {
					t.Fatal("duplicate addition succeeded")
				}
				PrintError(err, &stderr)
				if got, want := stderr.String(), "error: task already exists (1)\n"; got != want {
					t.Fatalf("stderr = %q, want %q", got, want)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if stdout.String() != test.wantNew {
				t.Fatalf("add output = %q, want %q", stdout.String(), test.wantNew)
			}
			stdout.Reset()
			if err := Execute([]string{"list"}, "test", &stdout); err != nil {
				t.Fatal(err)
			}
			if got, want := stdout.String(), "1 Fix login +done\n2 Other task\n"+test.wantNew; got != want {
				t.Fatalf("stored tasks = %q, want %q", got, want)
			}
		})
	}
}
