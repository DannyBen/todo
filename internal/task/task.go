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
	groups [][]filterPredicate
}

type filterKind uint8

const (
	termFilter filterKind = iota
	idFilter
	includeTagFilter
	excludeTagFilter
	includeReferenceFilter
	excludeReferenceFilter
)

type filterPredicate struct {
	kind  filterKind
	value string
	id    int64
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
		if literal, ok := unescapeLiteralToken(arg); ok {
			words = append(words, literal)
			continue
		}
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
	for _, token := range tokens {
		parts := strings.Split(token, "/")
		group := make([]filterPredicate, 0, len(parts))
		for _, part := range parts {
			if part == "" {
				return Filters{}, fmt.Errorf("invalid OR filter %q", token)
			}
			if literal, ok := unescapeLiteralToken(part); ok {
				group = append(group, filterPredicate{kind: termFilter, value: strings.ToLower(literal)})
				continue
			}
			predicate, err := parseFilterPredicate(part)
			if err != nil {
				return Filters{}, err
			}
			group = append(group, predicate)
		}
		filters.groups = append(filters.groups, group)
	}
	return filters, nil
}

func (filters Filters) Match(item Task) bool {
	description := strings.ToLower(item.Description)
	for _, group := range filters.groups {
		matched := false
		for _, predicate := range group {
			if predicate.match(item, description) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func parseFilterPredicate(value string) (filterPredicate, error) {
	parsed := classifyToken(value)
	switch parsed.kind {
	case removeReferenceToken:
		return filterPredicate{kind: excludeReferenceFilter, id: parsed.reference}, nil
	case addReferenceToken:
		return filterPredicate{kind: includeReferenceFilter, id: parsed.reference}, nil
	case addTagToken:
		return filterPredicate{kind: includeTagFilter, value: parsed.tag}, nil
	case removeTagToken:
		return filterPredicate{kind: excludeTagFilter, value: parsed.tag}, nil
	}
	if strings.HasPrefix(value, "=") {
		return filterPredicate{}, fmt.Errorf("filter prefix = is reserved")
	}
	if allDigits(value) {
		id, err := strconv.ParseInt(value, 10, 64)
		if err != nil || id < 1 {
			return filterPredicate{}, fmt.Errorf("invalid task ID %q", value)
		}
		return filterPredicate{kind: idFilter, id: id}, nil
	}
	return filterPredicate{kind: termFilter, value: strings.ToLower(value)}, nil
}

func (predicate filterPredicate) match(item Task, description string) bool {
	switch predicate.kind {
	case idFilter:
		return item.ID == predicate.id
	case includeTagFilter:
		return containsString(item.Tags, predicate.value)
	case excludeTagFilter:
		return !containsString(item.Tags, predicate.value)
	case includeReferenceFilter:
		return containsID(item.References, predicate.id)
	case excludeReferenceFilter:
		return !containsID(item.References, predicate.id)
	default:
		return strings.Contains(description, predicate.value)
	}
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
	parts := strings.Fields(item.Description)
	for index, part := range parts {
		_, alreadyEscaped := unescapeLiteralToken(part)
		if alreadyEscaped || classifyToken(part).kind != textToken {
			parts[index] = ":" + part
		}
	}
	tags := append([]string(nil), item.Tags...)
	sort.Strings(tags)
	for _, tag := range tags {
		parts = append(parts, "+"+tag)
	}
	references := append([]int64(nil), item.References...)
	sort.Slice(references, func(i, j int) bool { return references[i] < references[j] })
	for _, id := range references {
		parts = append(parts, "+"+strconv.FormatInt(id, 10))
	}
	return strings.Join(parts, " ")
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

func unescapeLiteralToken(value string) (string, bool) {
	index := 0
	for index < len(value) && value[index] == ':' {
		index++
	}
	if index == 0 || index == len(value) || (value[index] != '+' && value[index] != '-') {
		return "", false
	}
	return value[1:], true
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
