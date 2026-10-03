package cmd

import (
	"bytes"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"
)

func TestAddInput(t *testing.T) {
	for _, test := range []struct {
		name, input, want, wantError string
	}{
		{"tasks", "First +ready\r\n\nSecond :+literal :-h +1\r\nLast -ready", "2 First +ready\n3 Second +literal -h +1\n4 Last\n", ""},
		{"empty", " \n\t\r\n", "", ""},
		{"missing description", "First\n\n+ready\nLast", "", "line 3: task description cannot be empty"},
		{"missing connection", "First\n\nSecond +999\nLast", "", "line 3: connection target 999"},
		{"batch connection", "First\nSecond +2", "", "line 2: connection target 2"},
		{"long line", strings.Repeat("x", 70000), "2 " + strings.Repeat("x", 70000) + "\n", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("TODO_FILE", filepath.Join(t.TempDir(), "tasks.sqlite"))
			t.Setenv("TODO_BACKUP", "")
			var stdout bytes.Buffer
			if err := Execute([]string{"add", "Existing"}, "test", &stdout); err != nil {
				t.Fatal(err)
			}
			stdout.Reset()
			err := ExecuteWithIO([]string{"add"}, "test", strings.NewReader(test.input), &stdout, io.Discard)
			if test.wantError == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), test.wantError) || !strings.Contains(err.Error(), "no tasks added") {
				t.Fatalf("error = %v, want %q and no tasks added", err, test.wantError)
			}
			if stdout.String() != test.want {
				t.Fatalf("unexpected batch output: length %d, want %d", stdout.Len(), len(test.want))
			}
			stdout.Reset()
			if err := Execute([]string{"list"}, "test", &stdout); err != nil {
				t.Fatal(err)
			}
			wantExisting := "1 Existing\n"
			if test.name == "tasks" {
				wantExisting = "1 Existing +3\n"
			}
			if stdout.String() != wantExisting+test.want {
				t.Fatal("stored tasks differ from expected batch")
			}
		})
	}
}

type brokenAddIO struct{}

func (brokenAddIO) Read([]byte) (int, error)  { return 0, errors.New("broken input") }
func (brokenAddIO) Write([]byte) (int, error) { return 0, errors.New("broken output") }

func TestAddInputReadFailure(t *testing.T) {
	t.Setenv("TODO_FILE", filepath.Join(t.TempDir(), "tasks.sqlite"))
	t.Setenv("TODO_BACKUP", "")
	var stdout bytes.Buffer
	input := io.MultiReader(strings.NewReader("First\n"), brokenAddIO{})
	err := ExecuteWithIO([]string{"add"}, "test", input, &stdout, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "line 2: read input: broken input; no tasks added") {
		t.Fatalf("error = %v", err)
	}
	if err := Execute([]string{"list"}, "test", &stdout); err != nil {
		t.Fatal(err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("unexpected saved tasks or output: %q", stdout.String())
	}
}

func TestAddInputOutputFailure(t *testing.T) {
	t.Setenv("TODO_FILE", filepath.Join(t.TempDir(), "tasks.sqlite"))
	t.Setenv("TODO_BACKUP", "")
	err := ExecuteWithIO([]string{"add"}, "test", strings.NewReader("First\nSecond"), brokenAddIO{}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "tasks saved, but writing output failed") {
		t.Fatalf("error = %v", err)
	}
	var stdout bytes.Buffer
	if err := Execute([]string{"list"}, "test", &stdout); err != nil {
		t.Fatal(err)
	}
	if stdout.String() != "1 First\n2 Second\n" {
		t.Fatalf("saved tasks = %q", stdout.String())
	}
}

func TestAddArgumentsIgnoreInput(t *testing.T) {
	t.Setenv("TODO_FILE", filepath.Join(t.TempDir(), "tasks.sqlite"))
	t.Setenv("TODO_BACKUP", "")
	var stdout bytes.Buffer
	if err := ExecuteWithIO([]string{"add", "From arguments"}, "test", brokenAddIO{}, &stdout, io.Discard); err != nil {
		t.Fatal(err)
	}
	if stdout.String() != "1 From arguments\n" {
		t.Fatalf("output = %q", stdout.String())
	}
}
