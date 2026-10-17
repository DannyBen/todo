package cmd

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestTagEditSelection(t *testing.T) {
	const original = "1 First +v4\n2 Second +v4 +v5\n3 Third +done +v5\n4 Fourth\n"
	for _, test := range []struct {
		args []string
		want string
		err  string
	}{
		{args: []string{"+v4", "+release"}, want: "1 First +release +v4\n2 Second +release +v4 +v5\n"},
		{args: []string{"-done", "+done"}, want: "1 First +done +v4\n2 Second +done +v4 +v5\n4 Fourth +done\n"},
		{args: []string{"+v4/+v5", "-v4", "-v5", "+next"}, want: "1 First +next\n2 Second +next\n3 Third +done +next\n"},
		{args: []string{"4/+v4/1..2", "+next"}, want: "4 Fourth +next\n1 First +next +v4\n2 Second +next +v4 +v5\n"},
		{args: []string{"+done", "+next"}, want: "3 Third +done +next +v5\n"},
		{args: []string{"+missing/+v4", "+next"}, want: "1 First +next +v4\n2 Second +next +v4 +v5\n"},
		{args: []string{"+v4", "-done", "+release"}, want: "1 First +release +v4\n2 Second +release +v4 +v5\n"},
		{args: []string{"+v4"}, err: "require an edit expression"},
		{args: []string{"+done", "replacement"}, err: "bulk edits cannot replace"},
		{args: []string{"-done", "replacement"}, err: "bulk edits cannot replace"},
		{args: []string{"+missing", "+next"}, err: "no tasks match selection"},
		{args: []string{"+v4/99", "+next"}, err: "task 99 not found"},
		{args: []string{"+v4", "+next", "+99"}, err: "task 99 not found"},
		{args: []string{"v4", "+next"}, err: "invalid task ID"},
		{args: []string{"+v4 -done", "+next"}, err: "invalid task ID"},
		{args: []string{"+v4/", "+next"}, err: "invalid task ID"},
	} {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			t.Setenv("TODO_FILE", filepath.Join(t.TempDir(), "tasks.sqlite"))
			t.Setenv("TODO_BACKUP", "")
			for _, args := range [][]string{
				{"add", "First", "+v4"},
				{"add", "Second", "+v4", "+v5"},
				{"add", "Third", "+v5", "+done"},
				{"add", "Fourth"},
			} {
				if err := Execute(args, "test", &bytes.Buffer{}); err != nil {
					t.Fatal(err)
				}
			}
			var stdout bytes.Buffer
			err := Execute(append([]string{"edit"}, test.args...), "test", &stdout)
			if test.err == "" {
				if err != nil || stdout.String() != test.want {
					t.Fatalf("err=%v output=%q, want %q", err, stdout.String(), test.want)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.err) || stdout.Len() != 0 {
				t.Fatalf("err=%v output=%q, want error containing %q", err, stdout.String(), test.err)
			}
			if err := Execute([]string{"list"}, "test", &stdout); err != nil {
				t.Fatal(err)
			}
			if stdout.String() != original {
				t.Fatalf("failed edit mutated tasks: %q", stdout.String())
			}
		})
	}
}
