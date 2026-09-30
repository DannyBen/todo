package task

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type Task struct {
	ID          int64
	Description string
	Tags        []string
	References  []int64
}

type Change struct {
	Description      *string
	AddTags          []string
	RemoveTags       []string
	AddReferences    []int64
	RemoveReferences []int64
}

type Filters struct {
	Terms             []string
	IncludeTags       []string
	ExcludeTags       []string
	IncludeReferences []int64
	ExcludeReferences []int64
	ID                *int64
}

func ParseChange(args []string) (Change, error) {
	var change Change
	var words []string

	for _, arg := range args {
		removedReference, removesReference := parseRemovedReference(arg)
		removedTag, removesTag := parseRemovedTag(arg)
		switch {
		case removesReference:
			change.RemoveReferences = append(change.RemoveReferences, removedReference)
		case strings.HasPrefix(arg, "@"):
			id, err := parseReference(arg[1:], arg)
			if err != nil {
				return Change{}, err
			}
			change.AddReferences = append(change.AddReferences, id)
		case strings.HasPrefix(arg, "+"):
			tag, err := parseTag(arg[1:], arg)
			if err != nil {
				return Change{}, err
			}
			change.AddTags = append(change.AddTags, tag)
		case removesTag:
			change.RemoveTags = append(change.RemoveTags, removedTag)
		default:
			words = append(words, arg)
		}
	}

	if len(words) > 0 {
		description := strings.Join(words, " ")
		change.Description = &description
	}
	change.AddTags = uniqueStrings(change.AddTags)
	change.RemoveTags = uniqueStrings(change.RemoveTags)
	change.AddReferences = uniqueIDs(change.AddReferences)
	change.RemoveReferences = uniqueIDs(change.RemoveReferences)
	return change, nil
}

func ParseFilters(args []string) (Filters, error) {
	var filters Filters
	for _, arg := range args {
		removedReference, removesReference := parseRemovedReference(arg)
		removedTag, removesTag := parseRemovedTag(arg)
		switch {
		case removesReference:
			filters.ExcludeReferences = append(filters.ExcludeReferences, removedReference)
		case strings.HasPrefix(arg, "@"):
			id, err := parseReference(arg[1:], arg)
			if err != nil {
				return Filters{}, err
			}
			filters.IncludeReferences = append(filters.IncludeReferences, id)
		case strings.HasPrefix(arg, "+"):
			tag, err := parseTag(arg[1:], arg)
			if err != nil {
				return Filters{}, err
			}
			filters.IncludeTags = append(filters.IncludeTags, tag)
		case removesTag:
			filters.ExcludeTags = append(filters.ExcludeTags, removedTag)
		case strings.HasPrefix(arg, "="):
			return Filters{}, fmt.Errorf("filter prefix = is reserved")
		default:
			if id, err := strconv.ParseInt(arg, 10, 64); err == nil && id > 0 {
				if filters.ID != nil {
					return Filters{}, fmt.Errorf("only one task ID filter is allowed")
				}
				filters.ID = &id
			} else {
				filters.Terms = append(filters.Terms, strings.ToLower(arg))
			}
		}
	}
	return filters, nil
}

func (filters Filters) Match(item Task) bool {
	if filters.ID != nil && item.ID != *filters.ID {
		return false
	}
	description := strings.ToLower(item.Description)
	for _, term := range filters.Terms {
		if !strings.Contains(description, term) {
			return false
		}
	}
	for _, tag := range filters.IncludeTags {
		if !containsString(item.Tags, tag) {
			return false
		}
	}
	for _, tag := range filters.ExcludeTags {
		if containsString(item.Tags, tag) {
			return false
		}
	}
	for _, id := range filters.IncludeReferences {
		if !containsID(item.References, id) {
			return false
		}
	}
	for _, id := range filters.ExcludeReferences {
		if containsID(item.References, id) {
			return false
		}
	}
	return true
}

func Format(item Task) string {
	return format(item, 0, 0, false)
}

func FormatColor(item Task) string {
	return format(item, 0, 0, true)
}

func FormatWrapped(item Task, width, indent int, color bool) string {
	return format(item, width, indent, color)
}

type displayToken struct {
	plain  string
	styled string
}

func format(item Task, width, indent int, color bool) string {
	id := strconv.FormatInt(item.ID, 10)
	styledID := id
	if color {
		styledID = "\x1b[1;33m" + id + "\x1b[0m"
	}
	tokens := []displayToken{{plain: id, styled: styledID}}
	for _, word := range strings.Fields(item.Description) {
		tokens = append(tokens, displayToken{plain: word, styled: word})
	}
	tags := append([]string(nil), item.Tags...)
	sort.Strings(tags)
	for _, tag := range tags {
		formatted := "+" + tag
		styled := formatted
		if color {
			styled = "\x1b[1;34m" + formatted + "\x1b[0m"
		}
		tokens = append(tokens, displayToken{plain: formatted, styled: styled})
	}
	references := append([]int64(nil), item.References...)
	sort.Slice(references, func(i, j int) bool { return references[i] < references[j] })
	for _, id := range references {
		formatted := "@" + strconv.FormatInt(id, 10)
		tokens = append(tokens, displayToken{plain: formatted, styled: formatted})
	}

	if width <= 0 {
		parts := make([]string, len(tokens))
		for index, token := range tokens {
			parts[index] = token.styled
		}
		return strings.Join(parts, " ")
	}

	if indent < 1 {
		indent = len(id) + 1
	}
	continuation := strings.Repeat(" ", indent)
	var result strings.Builder
	lineWidth := 0
	for index, token := range tokens {
		separatorWidth := 1
		if index == 1 {
			separatorWidth = max(1, indent-len(id))
		}
		if index > 0 && lineWidth+separatorWidth+len(token.plain) > width {
			result.WriteByte('\n')
			result.WriteString(continuation)
			lineWidth = len(continuation)
		} else if index > 0 {
			result.WriteString(strings.Repeat(" ", separatorWidth))
			lineWidth += separatorWidth
		}
		result.WriteString(token.styled)
		lineWidth += len(token.plain)
	}
	return result.String()
}

func parseTag(value, original string) (string, error) {
	if !validTag(value) {
		return "", fmt.Errorf("invalid tag operation %q", original)
	}
	return value, nil
}

func validTag(value string) bool {
	return value != "" && !strings.ContainsRune("+-@=", rune(value[0]))
}

func parseRemovedTag(value string) (string, bool) {
	if !strings.HasPrefix(value, "-") || strings.HasPrefix(value, "-@") {
		return "", false
	}
	tag := value[1:]
	return tag, validTag(tag)
}

func parseRemovedReference(value string) (int64, bool) {
	if !strings.HasPrefix(value, "-@") {
		return 0, false
	}
	id, err := strconv.ParseInt(value[2:], 10, 64)
	return id, err == nil && id > 0
}

func parseReference(value, original string) (int64, error) {
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id < 1 {
		return 0, fmt.Errorf("invalid task reference %q", original)
	}
	return id, nil
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func uniqueIDs(values []int64) []int64 {
	seen := make(map[int64]bool, len(values))
	result := make([]int64, 0, len(values))
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func containsID(values []int64, wanted int64) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
