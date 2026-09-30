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
	IDs               []int64
}

type tokenKind uint8

const (
	textToken tokenKind = iota
	addTagToken
	removeTagToken
	addReferenceToken
	removeReferenceToken
)

type parsedToken struct {
	kind      tokenKind
	tag       string
	reference int64
}

func ParseChange(args []string) (Change, error) {
	var change Change
	var words []string
	var tokens []string
	for _, arg := range args {
		tokens = append(tokens, strings.Fields(arg)...)
	}

	for _, arg := range tokens {
		parsed := classifyToken(arg)
		switch parsed.kind {
		case removeReferenceToken:
			change.RemoveReferences = append(change.RemoveReferences, parsed.reference)
		case addReferenceToken:
			change.AddReferences = append(change.AddReferences, parsed.reference)
		case addTagToken:
			change.AddTags = append(change.AddTags, parsed.tag)
		case removeTagToken:
			change.RemoveTags = append(change.RemoveTags, parsed.tag)
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
	var tokens []string
	for _, arg := range args {
		tokens = append(tokens, strings.Fields(arg)...)
	}
	for _, arg := range tokens {
		parsed := classifyToken(arg)
		switch parsed.kind {
		case removeReferenceToken:
			filters.ExcludeReferences = append(filters.ExcludeReferences, parsed.reference)
		case addReferenceToken:
			filters.IncludeReferences = append(filters.IncludeReferences, parsed.reference)
		case addTagToken:
			filters.IncludeTags = append(filters.IncludeTags, parsed.tag)
		case removeTagToken:
			filters.ExcludeTags = append(filters.ExcludeTags, parsed.tag)
		case textToken:
			if strings.HasPrefix(arg, "=") {
				return Filters{}, fmt.Errorf("filter prefix = is reserved")
			}
			if ids, ok := ParseIDs(arg); ok {
				if len(filters.IDs) > 0 {
					return Filters{}, fmt.Errorf("only one task ID filter is allowed")
				}
				filters.IDs = ids
			} else {
				filters.Terms = append(filters.Terms, strings.ToLower(arg))
			}
		}
	}
	return filters, nil
}

func (filters Filters) Match(item Task) bool {
	if len(filters.IDs) > 0 && !containsID(filters.IDs, item.ID) {
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

func ParseIDs(value string) ([]int64, bool) {
	parts := strings.Split(value, "/")
	ids := make([]int64, 0, len(parts))
	for _, part := range parts {
		if !allDigits(part) {
			return nil, false
		}
		id, err := strconv.ParseInt(part, 10, 64)
		if err != nil || id < 1 {
			return nil, false
		}
		ids = append(ids, id)
	}
	return uniqueIDs(ids), true
}

func Format(item Task) string {
	return format(item, 0, 0, false)
}

func Expression(item Task) string {
	formatted := Format(item)
	_, expression, _ := strings.Cut(formatted, " ")
	return expression
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
		formatted := "+" + strconv.FormatInt(id, 10)
		styled := formatted
		if color {
			styled = "\x1b[1;35m" + formatted + "\x1b[0m"
		}
		tokens = append(tokens, displayToken{plain: formatted, styled: styled})
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

func classifyToken(value string) parsedToken {
	if len(value) < 2 || (value[0] != '+' && value[0] != '-') {
		return parsedToken{kind: textToken}
	}

	operand := value[1:]
	if allDigits(operand) {
		reference, err := strconv.ParseInt(operand, 10, 64)
		if err != nil || reference < 1 {
			return parsedToken{kind: textToken}
		}
		if value[0] == '+' {
			return parsedToken{kind: addReferenceToken, reference: reference}
		}
		return parsedToken{kind: removeReferenceToken, reference: reference}
	}

	if validTag(operand) {
		if value[0] == '+' {
			return parsedToken{kind: addTagToken, tag: operand}
		}
		return parsedToken{kind: removeTagToken, tag: operand}
	}
	return parsedToken{kind: textToken}
}

func validTag(value string) bool {
	if value == "" || !asciiLetterOrDigit(value[0]) {
		return false
	}
	for index := 1; index < len(value); index++ {
		if !asciiLetterOrDigit(value[index]) && value[index] != '-' && value[index] != '_' {
			return false
		}
	}
	return true
}

func allDigits(value string) bool {
	if value == "" {
		return false
	}
	for index := range len(value) {
		if value[index] < '0' || value[index] > '9' {
			return false
		}
	}
	return true
}

func asciiLetterOrDigit(value byte) bool {
	return value >= 'a' && value <= 'z' ||
		value >= 'A' && value <= 'Z' ||
		value >= '0' && value <= '9'
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
