package task

import (
	"fmt"
	"strings"
)

// Selection selects explicit IDs, inclusive ranges, and connections, joined with OR.
type Selection struct {
	parts []filterPredicate
	bulk  bool
}

func ParseSelection(value string) (Selection, error) {
	parts := selectionParts(value)
	selection := Selection{bulk: len(parts) > 1}
	for _, part := range parts {
		predicate, err := parseFilterPredicate(part)
		if err != nil {
			return Selection{}, err
		}
		if predicate.kind != idFilter && predicate.kind != rangeFilter && predicate.kind != includeReferenceFilter {
			return Selection{}, fmt.Errorf("invalid task ID %q", value)
		}
		selection.parts = append(selection.parts, predicate)
		selection.bulk = selection.bulk || predicate.kind != idFilter
	}
	return selection, nil
}

func selectionParts(value string) []string {
	var parts []string
	for _, part := range strings.Split(value, "/") {
		if id, ok := strings.CutSuffix(part, "+"); ok && allDigits(id) {
			parts = append(parts, id, "+"+id)
		} else {
			parts = append(parts, part)
		}
	}
	return parts
}

func (selection Selection) Bulk() bool {
	return selection.bulk
}

// Resolve preserves selector order, skips range gaps, and requires explicit IDs.
func (selection Selection) Resolve(items []Task) ([]int64, error) {
	var ids []int64
	for _, part := range selection.parts {
		found := false
		for _, item := range items {
			if part.match(item, "") {
				ids = append(ids, item.ID)
				found = true
			}
		}
		if !found && part.kind == idFilter {
			return nil, fmt.Errorf("task %d not found", part.id)
		}
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("no tasks match selection")
	}
	return uniqueIDs(ids), nil
}

func rangeToken(value string) bool {
	return strings.Contains(value, "..") &&
		(strings.HasPrefix(value, "..") || value[0] >= '0' && value[0] <= '9')
}

func parseRange(value string) (filterPredicate, error) {
	parts := strings.Split(value, "..")
	invalid := fmt.Errorf("invalid task range %q", value)
	if len(parts) != 2 || parts[0] == "" && parts[1] == "" {
		return filterPredicate{}, invalid
	}
	predicate := filterPredicate{kind: rangeFilter, id: 1}
	for index, part := range parts {
		if part == "" {
			continue
		}
		ids, ok := ParseIDs(part)
		if !ok || len(ids) != 1 {
			return filterPredicate{}, invalid
		}
		if index == 0 {
			predicate.id = ids[0]
		} else {
			predicate.end = ids[0]
		}
	}
	if predicate.end != 0 && predicate.id > predicate.end {
		return filterPredicate{}, invalid
	}
	return predicate, nil
}

func (filters Filters) HasRanges() bool {
	for _, group := range filters.groups {
		for _, predicate := range group {
			if predicate.kind == rangeFilter {
				return true
			}
		}
	}
	return false
}
