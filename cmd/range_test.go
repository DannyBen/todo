package cmd

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestRangeCommands(t *testing.T) {
	for _, test := range []struct {
		args []string
		want string
	}{
		{[]string{"ls", "1..3"}, "1 First\n3 Third\n"},
		{[]string{"ls", "..2"}, "1 First\n"},
		{[]string{"ls", "3.."}, "3 Third\n4 Fourth\n"},
		{[]string{"ls", "8.."}, ""},
		{[]string{"edit", "1..3", "+maybe", "-consider"}, "1 First +maybe\n3 Third +maybe\n"},
		{[]string{"edit", "1..3/4/3", "+ready"}, "1 First +ready\n3 Third +ready\n4 Fourth +ready\n"},
		{[]string{"edit", "3..", "+ready"}, "3 Third +ready\n4 Fourth +ready\n"},
		{[]string{"edit", "..2", "+ready"}, "1 First +ready\n"},
		{[]string{"edit", "1..3", "+4"}, "1 First +4\n3 Third +4\n"},
		{[]string{"del", "1..3"}, "1 First\n3 Third\n"},
		{[]string{"del", "..2"}, "1 First\n"},
		{[]string{"del", "3.."}, "3 Third\n4 Fourth\n"},
		{[]string{"del", "1..3/4/3"}, "1 First\n3 Third\n4 Fourth\n"},
		{[]string{"del", "1..4", "Third"}, "3 Third\n"},
	} {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			t.Setenv("TODO_FILE", filepath.Join(t.TempDir(), "tasks.sqlite"))
			for _, description := range []string{"First", "Second", "Third", "Fourth"} {
				if err := Execute([]string{"add", description}, "test", &bytes.Buffer{}); err != nil {
					t.Fatal(err)
				}
			}
			if err := Execute([]string{"del", "2"}, "test", &bytes.Buffer{}); err != nil {
				t.Fatal(err)
			}
			var stdout bytes.Buffer
			if err := Execute(test.args, "test", &stdout); err != nil {
				t.Fatal(err)
			}
			if stdout.String() != test.want {
				t.Fatalf("output = %q, want %q", stdout.String(), test.want)
			}
		})
	}
}

func TestBulkEditErrorsDoNotMutate(t *testing.T) {
	for _, test := range []struct {
		args []string
		err  string
	}{
		{[]string{"edit", "1..2", "replacement", "+changed"}, "bulk edits cannot replace"},
		{[]string{"edit", "1/2", "replacement"}, "bulk edits cannot replace"},
		{[]string{"edit", "1/1", "replacement"}, "bulk edits cannot replace"},
		{[]string{"edit", "1..1", "replacement"}, "bulk edits cannot replace"},
		{[]string{"edit", "..1", ":+literal"}, "bulk edits cannot replace"},
		{[]string{"edit", "1..1"}, "require an edit expression"},
		{[]string{"edit", "8..", "+ready"}, "no tasks match"},
		{[]string{"del", "8.."}, "no tasks match"},
		{[]string{"del", "1..2", "+missing"}, "no tasks match"},
		{[]string{"edit", "1..2/99", "+changed"}, "task 99 not found"},
		{[]string{"del", "1..2/99"}, "task 99 not found"},
		{[]string{"edit", "1..2", "+changed", "+99"}, "task 99 not found"},
	} {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			t.Setenv("TODO_FILE", filepath.Join(t.TempDir(), "tasks.sqlite"))
			for _, description := range []string{"First", "Second"} {
				if err := Execute([]string{"add", description}, "test", &bytes.Buffer{}); err != nil {
					t.Fatal(err)
				}
			}
			var stdout bytes.Buffer
			err := Execute(test.args, "test", &stdout)
			if err == nil || !strings.Contains(err.Error(), test.err) || stdout.Len() != 0 {
				t.Fatalf("err=%v stdout=%q", err, stdout.String())
			}
			if err := Execute([]string{"ls"}, "test", &stdout); err != nil {
				t.Fatal(err)
			}
			if stdout.String() != "1 First\n2 Second\n" {
				t.Fatalf("failed operation mutated tasks: %q", stdout.String())
			}
		})
	}
}
