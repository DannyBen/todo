package task

import (
	"reflect"
	"testing"
)

func TestRangeFilters(t *testing.T) {
	for _, test := range []struct {
		args []string
		want []int64
	}{
		{[]string{"2..4"}, []int64{2, 4}},
		{[]string{"..2"}, []int64{1, 2}},
		{[]string{"4.."}, []int64{4, 8}},
		{[]string{"2..4/8"}, []int64{2, 4, 8}},
		{[]string{"2..8", "+ready"}, []int64{4}},
		{[]string{"2..4/+ready"}, []int64{2, 4}},
		{[]string{"1..9223372036854775807"}, []int64{1, 2, 4, 8}},
		{[]string{"9.."}, nil},
	} {
		filters, err := ParseFilters(test.args)
		if err != nil {
			t.Fatal(err)
		}
		var got []int64
		for _, item := range []Task{{ID: 1}, {ID: 2}, {ID: 4, Tags: []string{"ready"}}, {ID: 8}} {
			if filters.Match(item) {
				got = append(got, item.ID)
			}
		}
		if !reflect.DeepEqual(got, test.want) {
			t.Errorf("filters %v matched %v, want %v", test.args, got, test.want)
		}
	}
	filters, err := ParseFilters([]string{"file..txt"})
	if err != nil || !filters.Match(Task{Description: "file..txt"}) {
		t.Fatalf("ordinary dotted text: %v", err)
	}
}

func TestInvalidRanges(t *testing.T) {
	for _, value := range []string{"..", "4..2", "0..2", "1..0", "1...2", "1..2..3", "1..x", "..x", "1..9223372036854775808"} {
		if _, err := ParseFilters([]string{value}); err == nil {
			t.Errorf("filter %q did not fail", value)
		}
		if _, err := ParseSelection(value); err == nil {
			t.Errorf("selection %q did not fail", value)
		}
	}
}

func TestSelectionResolve(t *testing.T) {
	items := []Task{{ID: 1}, {ID: 3}, {ID: 5}}
	for _, test := range []struct {
		value string
		want  []int64
		bulk  bool
	}{
		{"1", []int64{1}, false},
		{"1/1", []int64{1}, true},
		{"1..1", []int64{1}, true},
		{"1..5", []int64{1, 3, 5}, true},
		{"5/..3/3..5", []int64{5, 1, 3}, true},
		{"9../1", []int64{1}, true},
		{"3..", []int64{3, 5}, true},
	} {
		selection, err := ParseSelection(test.value)
		if err != nil {
			t.Fatal(err)
		}
		got, err := selection.Resolve(items)
		if err != nil || !reflect.DeepEqual(got, test.want) || selection.Bulk() != test.bulk {
			t.Errorf("selection %q: ids=%v bulk=%v err=%v", test.value, got, selection.Bulk(), err)
		}
	}
	for _, value := range []string{"9..", "2/1..5"} {
		selection, err := ParseSelection(value)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := selection.Resolve(items); err == nil {
			t.Errorf("unresolved selection %q did not fail", value)
		}
	}
}
