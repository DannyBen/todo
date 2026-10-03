package cmd

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestConnectedIDCommands(t *testing.T) {
	for _, test := range []struct {
		args []string
		want string
		err  string
	}{
		{args: []string{"ls", "1+"}, want: "1 First +2\n2 Second +1\n"},
		{args: []string{"ls", "1+/3"}, want: "1 First +2\n2 Second +1\n3 Third\n"},
		{args: []string{"edit", "1+", "+done"}, want: "1 First +done +2\n2 Second +done +1\n"},
		{args: []string{"edit", "1/+1", "+done"}, want: "1 First +done +2\n2 Second +done +1\n"},
		{args: []string{"del", "1+"}, want: "1 First +2\n2 Second +1\n"},
		{args: []string{"del", "1+", "Second"}, want: "2 Second +1\n"},
		{args: []string{"edit", "1+", "replacement"}, err: "bulk edits cannot replace"},
		{args: []string{"edit", "3+", "replacement"}, err: "bulk edits cannot replace"},
		{args: []string{"edit", "1+"}, err: "require an edit expression"},
		{args: []string{"edit", "9+", "+done"}, err: "task 9 not found"},
		{args: []string{"del", "9+"}, err: "task 9 not found"},
	} {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			t.Setenv("TODO_FILE", filepath.Join(t.TempDir(), "tasks.sqlite"))
			t.Setenv("TODO_BACKUP", "")
			for _, args := range [][]string{{"add", "First"}, {"add", "Second", "+1"}, {"add", "Third"}} {
				if err := Execute(args, "test", &bytes.Buffer{}); err != nil {
					t.Fatal(err)
				}
			}
			var stdout bytes.Buffer
			err := Execute(test.args, "test", &stdout)
			if test.err != "" {
				if err == nil || !strings.Contains(err.Error(), test.err) || stdout.Len() != 0 {
					t.Fatalf("err=%v output=%q", err, stdout.String())
				}
				if err := Execute([]string{"ls"}, "test", &stdout); err != nil {
					t.Fatal(err)
				}
				if stdout.String() != "1 First +2\n2 Second +1\n3 Third\n" {
					t.Fatalf("failed edit mutated tasks: %q", stdout.String())
				}
				return
			}
			if err != nil || stdout.String() != test.want {
				t.Fatalf("err=%v output=%q, want %q", err, stdout.String(), test.want)
			}
		})
	}
}
