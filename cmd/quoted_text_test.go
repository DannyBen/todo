package cmd

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestAddShellQuotedText(t *testing.T) {
	message := "this is a *quoted* test with # hash tags and (parens)"
	// The shell removes quotes. These cases represent the arguments it passes.
	for _, test := range []struct {
		name   string
		args   []string
		want   string
		tagged bool
	}{
		{"tag outside quotes", []string{"+test", message}, "1 " + message + " +test\n", true},
		{"tag inside quotes", []string{"+test " + message}, "1 " + message + " +test\n", true},
		{"literal inside quotes", []string{":+test " + message}, "1 +test " + message + "\n", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("TODO_FILE", filepath.Join(t.TempDir(), "tasks.sqlite"))
			t.Setenv("TODO_BACKUP", "")
			var stdout bytes.Buffer
			if err := Execute(append([]string{"add"}, test.args...), "test", &stdout); err != nil {
				t.Fatal(err)
			}
			if stdout.String() != test.want {
				t.Fatalf("add output = %q, want %q", stdout.String(), test.want)
			}
			stdout.Reset()
			if err := Execute([]string{"list"}, "test", &stdout); err != nil {
				t.Fatal(err)
			}
			if stdout.String() != test.want {
				t.Fatalf("stored task = %q, want %q", stdout.String(), test.want)
			}
			stdout.Reset()
			if err := Execute([]string{"list", "+test"}, "test", &stdout); err != nil {
				t.Fatal(err)
			}
			want := ""
			if test.tagged {
				want = test.want
			}
			if stdout.String() != want {
				t.Fatalf("tag filter output = %q, want %q", stdout.String(), want)
			}
		})
	}
}
