package task

import (
	"strings"
	"testing"
)

func TestParseChange(t *testing.T) {
	change, err := ParseChange([]string{"Prepare", "deployment", "+now", "-blocked", "+12", "-7"})
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

func TestParseChangeDistinguishesNumericReferencesFromTags(t *testing.T) {
	change, err := ParseChange([]string{"+42", "-7", "+release-2", "-under_score", "+123abc"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(change.AddTags, ","), "release-2,123abc"; got != want {
		t.Fatalf("added tags = %q, want %q", got, want)
	}
	if got, want := strings.Join(change.RemoveTags, ","), "under_score"; got != want {
		t.Fatalf("removed tags = %q, want %q", got, want)
	}
	if len(change.AddReferences) != 1 || change.AddReferences[0] != 42 {
		t.Fatalf("added references = %#v", change.AddReferences)
	}
	if len(change.RemoveReferences) != 1 || change.RemoveReferences[0] != 7 {
		t.Fatalf("removed references = %#v", change.RemoveReferences)
	}
}

func TestParseChangeNormalizesNewlines(t *testing.T) {
	change, err := ParseChange([]string{"first line\nsecond\tline", "+done"})
	if err != nil {
		t.Fatal(err)
	}
	if change.Description == nil || *change.Description != "first line second line" {
		t.Fatalf("description = %v", change.Description)
	}
}

func TestParseChangeDistinguishesTextFromRemovalSyntax(t *testing.T) {
	change, err := ParseChange([]string{"Write", "notes", "-", "then", "--", "review", "@12", "-draft", "-12", "+now"})
	if err != nil {
		t.Fatal(err)
	}
	if change.Description == nil {
		t.Fatal("description is nil")
	}
	if got, want := *change.Description, "Write notes - then -- review @12"; got != want {
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
	filters, err := ParseFilters([]string{"deploy", "+ready", "-blocked", "+7", "-9", "12"})
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

func TestFiltersUseORWithinSlashSeparatedIDs(t *testing.T) {
	filters, err := ParseFilters([]string{"7/12", "+ready"})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{7, 12} {
		if !filters.Match(Task{ID: id, Tags: []string{"ready"}}) {
			t.Fatalf("expected task %d to match", id)
		}
	}
	if filters.Match(Task{ID: 8, Tags: []string{"ready"}}) {
		t.Fatal("unexpected match for task 8")
	}
	if filters.Match(Task{ID: 7}) {
		t.Fatal("expected the tag filter to remain required")
	}
}

func TestFiltersUseORWithinTagGroups(t *testing.T) {
	filters, err := ParseFilters([]string{"+low/+maybe", "+active", "-defer"})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		tags []string
		want bool
	}{
		{name: "first alternative", tags: []string{"low", "active"}, want: true},
		{name: "second alternative", tags: []string{"maybe", "active"}, want: true},
		{name: "both alternatives", tags: []string{"low", "maybe", "active"}, want: true},
		{name: "missing required group", tags: []string{"active"}},
		{name: "missing separate filter", tags: []string{"low"}},
		{name: "excluded tag", tags: []string{"maybe", "active", "defer"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := filters.Match(Task{Tags: test.tags}); got != test.want {
				t.Fatalf("Match() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestFiltersUseORAcrossPredicateTypes(t *testing.T) {
	filters, err := ParseFilters([]string{"3/4/+low", "-defer"})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []Task{
		{ID: 3},
		{ID: 4},
		{ID: 9, Tags: []string{"low"}},
	} {
		if !filters.Match(item) {
			t.Fatalf("expected task %#v to match", item)
		}
	}
	if filters.Match(Task{ID: 9}) {
		t.Fatal("unexpected match without an OR alternative")
	}
	if filters.Match(Task{ID: 3, Tags: []string{"defer"}}) {
		t.Fatal("separate exclusion filter did not remain required")
	}
}

func TestParseIDs(t *testing.T) {
	ids, ok := ParseIDs("7/1/7")
	if !ok || len(ids) != 2 || ids[0] != 7 || ids[1] != 1 {
		t.Fatalf("ParseIDs() = %#v, %v", ids, ok)
	}
	for _, value := range []string{"+1", "1/", "/1", "1//2", "0", "one"} {
		if ids, ok := ParseIDs(value); ok {
			t.Fatalf("ParseIDs(%q) = %#v, true", value, ids)
		}
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
		{name: "missing reference", args: []string{"+9"}},
		{name: "excluded reference", args: []string{"-7"}},
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

func TestFiltersUseANDBetweenGroups(t *testing.T) {
	filters, err := ParseFilters([]string{"1", "2"})
	if err != nil {
		t.Fatal(err)
	}
	if filters.Match(Task{ID: 1}) || filters.Match(Task{ID: 2}) {
		t.Fatal("separate ID filters unexpectedly matched")
	}
}

func TestInvalidOperationsRemainPlainText(t *testing.T) {
	args := []string{"fix", "todo", "rm", "+", "tag", "@", "task", "@2", "+@2", "-@2", "+0", "-0", "-", "later"}
	want := "fix todo rm + tag @ task @2 +@2 -@2 +0 -0 - later"

	change, err := ParseChange(args)
	if err != nil {
		t.Fatal(err)
	}
	if change.Description == nil || *change.Description != want {
		t.Fatalf("description = %v, want %q", change.Description, want)
	}

	filters, err := ParseFilters(args)
	if err != nil {
		t.Fatal(err)
	}
	if !filters.Match(Task{Description: want}) {
		t.Fatalf("plain-text filters did not match %q", want)
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
	if got, want := Format(item), "12 Ship +a +z +2 +9"; got != want {
		t.Fatalf("Format() = %q, want %q", got, want)
	}
}

func TestFormatColorColorsIDsTagsAndReferences(t *testing.T) {
	item := Task{ID: 12, Description: "Ship", Tags: []string{"done"}, References: []int64{9}}
	if got, want := FormatColor(item), "\x1b[1;33m12\x1b[0m Ship \x1b[1;34m+done\x1b[0m \x1b[1;35m+9\x1b[0m"; got != want {
		t.Fatalf("FormatColor() = %q, want %q", got, want)
	}
}

func TestExpressionOmitsTaskID(t *testing.T) {
	item := Task{ID: 12, Description: "Ship it", Tags: []string{"done"}, References: []int64{9}}
	if got, want := Expression(item), "Ship it +done +9"; got != want {
		t.Fatalf("Expression() = %q, want %q", got, want)
	}
}

func TestFormatWrappedIndentsContinuationLines(t *testing.T) {
	item := Task{ID: 1, Description: "one two three", Tags: []string{"done"}, References: []int64{9}}
	want := "1  one two\n   three\n   +done +9"
	if got := FormatWrapped(item, 11, 3, false); got != want {
		t.Fatalf("FormatWrapped() = %q, want %q", got, want)
	}
}
