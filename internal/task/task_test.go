package task

import "testing"

func TestParseChange(t *testing.T) {
	change, err := ParseChange([]string{"Prepare", "deployment", "+now", "-blocked", "@12", "-@7"})
	if err != nil {
		t.Fatal(err)
	}
	if change.Description == nil || *change.Description != "Prepare deployment" {
		t.Fatalf("description = %v", change.Description)
	}
	if len(change.AddTags) != 1 || change.AddTags[0] != "now" {
		t.Fatalf("added tags = %#v", change.AddTags)
	}
	if len(change.RemoveTags) != 1 || change.RemoveTags[0] != "blocked" {
		t.Fatalf("removed tags = %#v", change.RemoveTags)
	}
	if len(change.AddReferences) != 1 || change.AddReferences[0] != 12 {
		t.Fatalf("added references = %#v", change.AddReferences)
	}
	if len(change.RemoveReferences) != 1 || change.RemoveReferences[0] != 7 {
		t.Fatalf("removed references = %#v", change.RemoveReferences)
	}
}

func TestParseChangeDistinguishesTextFromRemovalSyntax(t *testing.T) {
	change, err := ParseChange([]string{"Write", "notes", "-", "then", "--", "review", "-draft", "-@12", "+now"})
	if err != nil {
		t.Fatal(err)
	}
	if change.Description == nil {
		t.Fatal("description is nil")
	}
	if got, want := *change.Description, "Write notes - then -- review"; got != want {
		t.Fatalf("description = %q, want %q", got, want)
	}
	if len(change.AddTags) != 1 || change.AddTags[0] != "now" {
		t.Fatalf("added tags = %#v", change.AddTags)
	}
	if len(change.RemoveTags) != 1 || change.RemoveTags[0] != "draft" {
		t.Fatalf("removed tags = %#v", change.RemoveTags)
	}
	if len(change.RemoveReferences) != 1 || change.RemoveReferences[0] != 12 {
		t.Fatalf("removed references = %#v", change.RemoveReferences)
	}
}

func TestFiltersUseANDSemantics(t *testing.T) {
	filters, err := ParseFilters([]string{"deploy", "+ready", "-blocked", "@7", "-@9", "12"})
	if err != nil {
		t.Fatal(err)
	}
	matching := Task{ID: 12, Description: "Prepare Deployment", Tags: []string{"ready"}, References: []int64{7}}
	if !filters.Match(matching) {
		t.Fatal("expected task to match all filters")
	}
	matching.Tags = append(matching.Tags, "blocked")
	if filters.Match(matching) {
		t.Fatal("expected excluded tag to prevent match")
	}
}

func TestFiltersRejectNonMatches(t *testing.T) {
	item := Task{ID: 12, Description: "Prepare deployment", Tags: []string{"ready"}, References: []int64{7}}
	tests := []struct {
		name string
		args []string
	}{
		{name: "different ID", args: []string{"13"}},
		{name: "missing term", args: []string{"release"}},
		{name: "missing tag", args: []string{"+blocked"}},
		{name: "missing reference", args: []string{"@9"}},
		{name: "excluded reference", args: []string{"-@7"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			filters, err := ParseFilters(test.args)
			if err != nil {
				t.Fatal(err)
			}
			if filters.Match(item) {
				t.Fatalf("filters %#v unexpectedly matched %#v", test.args, item)
			}
		})
	}
}

func TestInvalidSyntax(t *testing.T) {
	tests := [][]string{
		{"+"},
		{"@bad"},
		{"@0"},
	}
	for _, args := range tests {
		if _, err := ParseChange(args); err == nil {
			t.Fatalf("ParseChange(%#v) did not return an error", args)
		}
	}

	if _, err := ParseFilters([]string{"1", "2"}); err == nil {
		t.Fatal("expected multiple ID filter error")
	}
}

func TestEqualFilterPrefixIsReserved(t *testing.T) {
	if _, err := ParseFilters([]string{"=12"}); err == nil {
		t.Fatal("expected reserved prefix error")
	}
}

func TestInvalidRemovalSyntaxIsAPlainTextFilter(t *testing.T) {
	filters, err := ParseFilters([]string{"-", "--"})
	if err != nil {
		t.Fatal(err)
	}
	if !filters.Match(Task{Description: "Explain - and -- syntax"}) {
		t.Fatal("expected standalone hyphens to be plain text filters")
	}
}

func TestFormatSortsTagsAndReferences(t *testing.T) {
	item := Task{ID: 12, Description: "Ship", Tags: []string{"z", "a"}, References: []int64{9, 2}}
	if got, want := Format(item), "12 Ship +a +z @2 @9"; got != want {
		t.Fatalf("Format() = %q, want %q", got, want)
	}
}

func TestFormatColorColorsIDsAndTags(t *testing.T) {
	item := Task{ID: 12, Description: "Ship", Tags: []string{"done"}, References: []int64{9}}
	if got, want := FormatColor(item), "\x1b[33m12\x1b[0m Ship \x1b[1;35m+done\x1b[0m @9"; got != want {
		t.Fatalf("FormatColor() = %q, want %q", got, want)
	}
}

func TestFormatWrappedIndentsContinuationLines(t *testing.T) {
	item := Task{ID: 1, Description: "one two three", Tags: []string{"done"}, References: []int64{9}}
	want := "1  one two\n   three\n   +done @9"
	if got := FormatWrapped(item, 11, 3, false); got != want {
		t.Fatalf("FormatWrapped() = %q, want %q", got, want)
	}
}
