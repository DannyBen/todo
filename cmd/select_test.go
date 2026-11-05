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
		t.Run(tc.name, func(t *testing.T) {
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
			err := executeWithInteraction([]string{tc.command}, "test", strings.NewReader(""), &stdout, &stderr, prompt, selectPrompt)
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

func TestSelectionNeedsTerminal(t *testing.T) {
	t.Setenv("TODO_FILE", filepath.Join(t.TempDir(), "tasks.sqlite"))
	if err := Execute([]string{"add", "Keep"}, "test", &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"del", "edit"} {
		var stdout bytes.Buffer
		err := ExecuteWithIO([]string{command}, "test", strings.NewReader(""), &stdout, &bytes.Buffer{})
		if err == nil || !strings.Contains(err.Error(), "interactive selection requires a terminal") || stdout.Len() != 0 {
			t.Fatalf("%s: error = %v, stdout = %q", command, err, stdout.String())
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
		text string
		cols int
		want string
	}{
		{"abcdef", 3, "abc"},
		{"界ab", 3, "界a"},
		{"e\u0301ab", 2, "e\u0301a"},
		{"a\n\r\tb\x1b[H", 20, "ab[H"},
		{"abc", 0, ""},
	} {
		if got := selectorText(tc.text, tc.cols); got != tc.want {
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
