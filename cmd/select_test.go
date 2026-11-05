package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dannyben/todo/internal/task"
)

func TestSelectedCommands(t *testing.T) {
	for _, tc := range []struct {
		name, command, expression, want, errorText string
		ids                                        []int64
		cancel                                     bool
	}{
		{name: "delete", command: "del", ids: []int64{1, 3}, want: "2 Second +old\n"},
		{name: "delete alias", command: "rm", ids: []int64{2}, want: "1 First +old\n3 Third +old\n"},
		{name: "edit one", command: "edit", ids: []int64{2}, expression: "Changed -old +new", want: "1 First +old\n2 Changed +new\n3 Third +old\n"},
		{name: "edit many", command: "e", ids: []int64{1, 3}, expression: "+new -old +2", want: "1 First +new +2\n2 Second +old +1 +3\n3 Third +new +2\n"},
		{name: "bulk description rejected", command: "edit", ids: []int64{1, 3}, expression: "New description +new", errorText: "bulk edits cannot replace task descriptions"},
		{name: "bad connection atomic", command: "edit", ids: []int64{1, 3}, expression: "+new +999", errorText: "task 999 not found"},
		{name: "abort delete", command: "del"},
		{name: "abort edit", command: "edit"},
		{name: "cancel single prompt", command: "edit", ids: []int64{1}, cancel: true},
		{name: "cancel bulk prompt", command: "edit", ids: []int64{1, 3}, cancel: true},
		{name: "empty bulk prompt", command: "edit", ids: []int64{1, 3}},
	} {
		filters := []string{""}
		if tc.command == "edit" || tc.command == "e" {
			filters = append(filters, "+old")
		}
		for _, filter := range filters {
			t.Run(tc.name+"/"+filter, func(t *testing.T) {
				t.Setenv("TODO_FILE", filepath.Join(t.TempDir(), "tasks.sqlite"))
				var stdout, stderr bytes.Buffer
				for _, name := range []string{"First", "Second", "Third"} {
					if err := Execute([]string{"add", name, "+old"}, "test", &stdout); err != nil {
						t.Fatal(err)
					}
				}
				stdout.Reset()
				selectPrompt := func(items []task.Task, command string, _ io.Reader, _ io.Writer) ([]int64, error) {
					if len(items) != 3 || command != "edit" && command != "del" {
						t.Fatalf("unexpected selector input: %v %s", items, command)
					}
					return tc.ids, nil
				}
				prompt := func(selector, current string, _ io.Reader, _ io.Writer) (string, error) {
					if len(tc.ids) == 1 && (selector != "1" && selector != "2" || current == "") {
						t.Fatalf("single prompt: %q %q", selector, current)
					}
					if len(tc.ids) > 1 && (selector != "1/3" || current != "") {
						t.Fatalf("bulk prompt: %q %q", selector, current)
					}
					if tc.cancel {
						return "", errEditCanceled
					}
					return tc.expression, nil
				}
				args := []string{tc.command}
				if filter != "" {
					args = append(args, filter)
				}
				err := executeWithInteraction(args, "test", strings.NewReader(""), &stdout, &stderr, prompt, selectPrompt)
				if tc.errorText != "" {
					if err == nil || !strings.Contains(err.Error(), tc.errorText) {
						t.Fatalf("error = %v, want %s", err, tc.errorText)
					}
				} else if err != nil {
					t.Fatal(err)
				}
				if tc.want == "" && stdout.Len() != 0 {
					t.Fatalf("canceled or failed action printed %q", stdout.String())
				}
				stdout.Reset()
				if err := Execute([]string{"list"}, "test", &stdout); err != nil {
					t.Fatal(err)
				}
				want := tc.want
				if want == "" {
					want = "1 First +old\n2 Second +old\n3 Third +old\n"
				}
				if stdout.String() != want {
					t.Fatalf("tasks = %q, want %q", stdout.String(), want)
				}
			})
		}
	}
}

func TestFilteredEditSelection(t *testing.T) {
	for _, tc := range []struct {
		filter string
		want   []int64
		err    string
	}{
		{filter: "+work", want: []int64{1, 2}},
		{filter: "-work", want: []int64{3, 4}},
		{filter: "2/1", want: []int64{1, 2}},
		{filter: "1..2", want: []int64{1, 2}},
		{filter: "..2", want: []int64{1, 2}},
		{filter: "3..", want: []int64{3, 4}},
		{filter: "+1", want: []int64{2}},
		{filter: "1+", want: []int64{1, 2}},
		{filter: "3/+work/1..2", want: []int64{1, 2, 3}},
		{filter: "+solo", want: []int64{3}},
		{filter: "1..1", want: []int64{1}},
		{filter: "1/1", want: []int64{1}},
		{filter: "+missing", err: "no tasks match selection"},
		{filter: "8..", err: "no tasks match selection"},
		{filter: "+work/99", err: "task 99 not found"},
		{filter: "99+", err: "task 99 not found"},
		{filter: "work", err: "invalid task ID"},
		{filter: "+work -solo", err: "invalid task ID"},
	} {
		t.Run(tc.filter, func(t *testing.T) {
			t.Setenv("TODO_FILE", filepath.Join(t.TempDir(), "tasks.sqlite"))
			for _, args := range [][]string{
				{"add", "First", "+work"},
				{"add", "Second", "+work", "+1"},
				{"add", "Third", "+solo"},
				{"add", "Fourth"},
			} {
				if err := Execute(args, "test", &bytes.Buffer{}); err != nil {
					t.Fatal(err)
				}
			}
			called := false
			selectPrompt := func(items []task.Task, command string, _ io.Reader, _ io.Writer) ([]int64, error) {
				called = true
				var ids []int64
				for _, item := range items {
					ids = append(ids, item.ID)
				}
				if command != "edit" || !reflect.DeepEqual(ids, tc.want) {
					t.Fatalf("selector = %s %v, want edit %v", command, ids, tc.want)
				}
				return nil, nil
			}
			prompt := func(string, string, io.Reader, io.Writer) (string, error) {
				t.Fatal("edit prompt opened after cancellation or invalid filter")
				return "", nil
			}
			var stdout, stderr bytes.Buffer
			err := executeWithInteraction([]string{"edit", tc.filter}, "test", strings.NewReader(""), &stdout, &stderr, prompt, selectPrompt)
			if tc.err != "" {
				if err == nil || !strings.Contains(err.Error(), tc.err) || called {
					t.Fatalf("error = %v, picker opened = %v, want %q before selection", err, called, tc.err)
				}
			} else if err != nil || !called {
				t.Fatalf("error = %v, picker opened = %v", err, called)
			}
			if stdout.Len() != 0 || stderr.Len() != 0 {
				t.Fatalf("unexpected output: stdout %q, stderr %q", stdout.String(), stderr.String())
			}
			if err := Execute([]string{"list"}, "test", &stdout); err != nil {
				t.Fatal(err)
			}
			if want := "1 First +work +2\n2 Second +work +1\n3 Third +solo\n4 Fourth\n"; stdout.String() != want {
				t.Fatalf("tasks changed: %q, want %q", stdout.String(), want)
			}
		})
	}
}

func TestSelectionNeedsTerminal(t *testing.T) {
	t.Setenv("TODO_FILE", filepath.Join(t.TempDir(), "tasks.sqlite"))
	if err := Execute([]string{"add", "Keep"}, "test", &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"del"}, {"edit"}, {"edit", "1..1"}} {
		var stdout bytes.Buffer
		err := ExecuteWithIO(args, "test", strings.NewReader(""), &stdout, &bytes.Buffer{})
		if err == nil || !strings.Contains(err.Error(), "interactive selection requires a terminal") || stdout.Len() != 0 {
			t.Fatalf("%v: error = %v, stdout = %q", args, err, stdout.String())
		}
		if args[0] == "edit" && !strings.Contains(err.Error(), "provide a filter and edit expression") {
			t.Fatalf("edit error lacks script instructions: %v", err)
		}
	}
}

func TestSelectorNavigation(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	s := taskSelector{items: []task.Task{{ID: 2}, {ID: 4}, {ID: 7}, {ID: 9}}, marked: make([]bool, 4)}
	s.move("up", 2)
	if s.cursor != 0 || len(s.ids()) != 0 {
		t.Fatal("initial selection should be empty and cursor should stay at start")
	}
	s.move("mark", 2)
	s.move("pagedown", 2)
	s.move("mark", 2)
	screen := s.render("del", 80, 3)
	if s.top != 1 || !strings.Contains(screen, "> [x] 7") || strings.Contains(screen, "[x] 2") {
		t.Fatalf("scrolled screen = %q, top = %d", screen, s.top)
	}
	if got := s.ids(); !reflect.DeepEqual(got, []int64{2, 7}) {
		t.Fatalf("marked IDs = %v", got)
	}
	s.move("mark", 2)
	s.move("end", 2)
	s.move("down", 2)
	if s.cursor != 3 {
		t.Fatal("cursor moved beyond end")
	}
	s.move("pageup", 2)
	s.move("home", 2)
	screen = s.render("edit", 80, 8)
	if s.cursor != 0 || s.top != 0 || !strings.Contains(screen, "> [x] 2") || !strings.Contains(screen, "Enter: confirm edit") {
		t.Fatalf("resized screen = %q", screen)
	}
}

func TestSelectorText(t *testing.T) {
	for _, tc := range []struct {
		text  string
		cols  int
		want  string
		color bool
	}{
		{"abcdef", 3, "abc", false},
		{"界ab", 3, "界a", false},
		{"e\u0301ab", 2, "e\u0301a", false},
		{"a\n\r\tb\x1b[H", 20, "ab[H", false},
		{"abc", 0, "", false},
		{"\x1b[1;34m界abc\x1b[0m", 3, "\x1b[1;34m界a\x1b[0m", true},
		{"\x1b[1mabc\x1b[H", 20, "\x1b[1mabc[H\x1b[0m", true},
	} {
		if got := selectorText(tc.text, tc.cols, tc.color); got != tc.want {
			t.Fatalf("selectorText(%q, %d) = %q, want %q", tc.text, tc.cols, got, tc.want)
		}
	}
}

func TestSelectionKeys(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"\x1b[A", "up"}, {"\x1bOB", "down"}, {"\x1b[H", "home"},
		{"\x1b[F", "end"}, {"\x1b[5~", "pageup"}, {"\x1b[6~", "pagedown"},
		{" ", "mark"}, {"\r", "enter"}, {"\n", "enter"},
		{"\x03", "abort"}, {"\x04", "abort"}, {"\x1b", "abort"}, {"q", "abort"},
		{"\x1b[C", ""}, {"\x1b[3~", ""}, {"x", ""},
	} {
		t.Run(tc.input, func(t *testing.T) {
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			defer w.Close()
			if _, err := io.WriteString(w, tc.input); err != nil {
				t.Fatal(err)
			}
			if got, err := selectionKey(r); err != nil || got != tc.want {
				t.Fatalf("key = %q, %v, want %q", got, err, tc.want)
			}
		})
	}
}
