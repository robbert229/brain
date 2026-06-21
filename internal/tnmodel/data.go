package tnmodel

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const dateLayout = "2006-01-02"

var (
	scheduledLayouts = []string{
		dateLayout,
		"2006-01-02T15:04",
		"2006-01-02T15:04:05",
		time.RFC3339,
		time.RFC3339Nano,
	}

	dateTimeLayouts = []string{
		dateLayout,
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.999-0700",
		"2006-01-02T15:04:05-0700",
		"2006-01-02T15:04-0700",
	}
)

// Common TaskNotes statuses.
const (
	StatusOpen      = "open"
	StatusClosed    = "closed"
	StatusCompleted = "completed"
	StatusDone      = "done"
	StatusNone      = "none"
)

// Common TaskNotes priorities.
const (
	PriorityLow    = "low"
	PriorityNormal = "normal"
	PriorityHigh   = "high"
)

// Decode parses a TaskNotes document (YAML frontmatter + markdown body).
func Decode(id string, content string) (*TaskNote, error) {
	content = strings.ReplaceAll(content, "\r\n", "\n")

	fmText, body, hasFrontmatter, err := splitFrontmatter(content)
	if err != nil {
		return nil, err
	}

	note := &TaskNote{
		Body: stringPtr(body),
		Frontmatter: TaskFrontmatter{
			Tags: []string{},
		},
	}
	if id != "" {
		note.File = &TaskNoteFile{Path: stringPtr(id)}
	}

	if hasFrontmatter {
		var raw map[string]any
		if err := yaml.Unmarshal([]byte(fmText), &raw); err != nil {
			return nil, fmt.Errorf("parse frontmatter: %w", err)
		}

		if value := stringField(raw, "status"); value != "" {
			note.Frontmatter.Status = value
		}
		if value := stringField(raw, "priority"); value != "" {
			note.Frontmatter.Priority = stringPtr(value)
		}
		if value := stringField(raw, "title"); value != "" {
			note.Frontmatter.Title = value
		}
		if value := stringField(raw, "scheduled"); value != "" {
			if _, err := parseWithLayouts(value, scheduledLayouts); err != nil {
				return nil, fmt.Errorf("parse scheduled: %w", err)
			}
			scheduled := DateOrDateTime(value)
			note.Frontmatter.Scheduled = &scheduled
		}
		if value := stringField(raw, "due"); value != "" {
			if _, err := parseWithLayouts(value, scheduledLayouts); err != nil {
				return nil, fmt.Errorf("parse due: %w", err)
			}
			due := DateOrDateTime(value)
			note.Frontmatter.Due = &due
		}
		if value := firstStringField(raw, "date_created", "dateCreated"); value != "" {
			t, err := parseWithLayouts(value, dateTimeLayouts)
			if err != nil {
				return nil, fmt.Errorf("parse dateCreated: %w", err)
			}
			note.Frontmatter.DateCreated = t
		}
		if value := firstStringField(raw, "date_modified", "dateModified"); value != "" {
			t, err := parseWithLayouts(value, dateTimeLayouts)
			if err != nil {
				return nil, fmt.Errorf("parse dateModified: %w", err)
			}
			note.Frontmatter.DateModified = t
		}
		if tags, ok := stringSliceField(raw, "tags"); ok {
			note.Frontmatter.Tags = tags
		}

		note.Frontmatter.AdditionalProperties = frontmatterExtra(raw)
	}

	if note.Frontmatter.Title == "" {
		note.Frontmatter.Title = extractFirstH1(body)
	}

	return note, nil
}

// Encode serializes a TaskNote into the TaskNotes format.
func Encode(note *TaskNote) (string, error) {
	if note == nil {
		return "", nil
	}

	fmNode, err := buildFrontmatterNode(note)
	if err != nil {
		return "", err
	}

	fmBytes, err := yaml.Marshal(fmNode)
	if err != nil {
		return "", fmt.Errorf("marshal frontmatter: %w", err)
	}

	body := Body(note)
	if body == "" && Title(note) != "" {
		body = "# " + Title(note) + "\n"
	}

	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString(string(fmBytes))
	b.WriteString("---\n")
	if body != "" {
		b.WriteString(body)
	}
	return b.String(), nil
}

func ID(note *TaskNote) string {
	if note == nil || note.File == nil || note.File.Path == nil {
		return ""
	}

	return *note.File.Path
}

func Body(note *TaskNote) string {
	if note == nil || note.Body == nil {
		return ""
	}

	return *note.Body
}

func Title(note *TaskNote) string {
	if note == nil {
		return ""
	}

	return note.Frontmatter.Title
}

func Status(note *TaskNote) string {
	if note == nil {
		return ""
	}

	return note.Frontmatter.Status
}

func Priority(note *TaskNote) string {
	if note == nil || note.Frontmatter.Priority == nil {
		return ""
	}

	return *note.Frontmatter.Priority
}

func Tags(note *TaskNote) []string {
	if note == nil {
		return nil
	}

	return note.Frontmatter.Tags
}

func Scheduled(note *TaskNote) *time.Time {
	if note == nil {
		return nil
	}

	return parseDateOrDateTime(note.Frontmatter.Scheduled)
}

func Due(note *TaskNote) *time.Time {
	if note == nil {
		return nil
	}

	return parseDateOrDateTime(note.Frontmatter.Due)
}

func parseDateOrDateTime(value *DateOrDateTime) *time.Time {
	if value == nil {
		return nil
	}

	t, err := parseWithLayouts(string(*value), scheduledLayouts)
	if err != nil {
		return nil
	}

	return &t
}

func splitFrontmatter(content string) (frontmatterText string, body string, hasFrontmatter bool, err error) {
	if !strings.HasPrefix(content, "---\n") {
		return "", content, false, nil
	}

	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return "", "", false, fmt.Errorf("invalid frontmatter start")
	}

	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			frontmatterText = strings.Join(lines[1:i], "\n")
			body = strings.Join(lines[i+1:], "\n")
			return frontmatterText, body, true, nil
		}
	}

	return "", "", false, fmt.Errorf("frontmatter is missing closing delimiter")
}

func extractFirstH1(body string) string {
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
		}
	}
	return ""
}

func buildFrontmatterNode(note *TaskNote) (*yaml.Node, error) {
	mapping := &yaml.Node{Kind: yaml.MappingNode}

	appendScalar := func(k, v string) {
		mapping.Content = append(mapping.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: k},
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v},
		)
	}

	fm := note.Frontmatter
	if fm.Title != "" {
		appendScalar("title", fm.Title)
	}
	if fm.Status != "" {
		appendScalar("status", fm.Status)
	}
	if fm.Priority != nil && *fm.Priority != "" {
		appendScalar("priority", *fm.Priority)
	}
	if fm.Scheduled != nil {
		appendScalar("scheduled", string(*fm.Scheduled))
	}
	if fm.Due != nil {
		appendScalar("due", string(*fm.Due))
	}
	if !fm.DateCreated.IsZero() {
		appendScalar("dateCreated", fm.DateCreated.Format(time.RFC3339Nano))
	}
	if !fm.DateModified.IsZero() {
		appendScalar("dateModified", fm.DateModified.Format(time.RFC3339Nano))
	}
	if len(fm.Tags) > 0 {
		mapping.Content = append(mapping.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "tags"},
			stringSliceNode(fm.Tags),
		)
	}

	extra, ok := fm.AdditionalProperties.(map[string]any)
	if ok {
		keys := make([]string, 0, len(extra))
		for k := range extra {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			node, err := anyToYAMLNode(extra[k])
			if err != nil {
				return nil, fmt.Errorf("marshal extra field %q: %w", k, err)
			}
			mapping.Content = append(mapping.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: k},
				node,
			)
		}
	}

	return mapping, nil
}

func stringSliceNode(values []string) *yaml.Node {
	n := &yaml.Node{Kind: yaml.SequenceNode}
	for _, v := range values {
		n.Content = append(n.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v})
	}

	return n
}

func anyToYAMLNode(v any) (*yaml.Node, error) {
	b, err := yaml.Marshal(v)
	if err != nil {
		return nil, err
	}

	var node yaml.Node
	if err := yaml.Unmarshal(b, &node); err != nil {
		return nil, err
	}

	if len(node.Content) == 0 {
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}, nil
	}

	return node.Content[0], nil
}

func stringPtr(value string) *string {
	return &value
}

func stringField(values map[string]any, key string) string {
	value, ok := values[key]
	if !ok {
		return ""
	}

	if t, ok := value.(time.Time); ok {
		if t.Hour() == 0 && t.Minute() == 0 && t.Second() == 0 && t.Nanosecond() == 0 {
			return t.Format(dateLayout)
		}

		return t.Format(time.RFC3339Nano)
	}

	return fmt.Sprint(value)
}

func firstStringField(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := stringField(values, key); value != "" {
			return value
		}
	}

	return ""
}

func stringSliceField(values map[string]any, key string) ([]string, bool) {
	value, ok := values[key]
	if !ok {
		return nil, false
	}

	items, ok := value.([]any)
	if !ok {
		return nil, false
	}

	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, fmt.Sprint(item))
	}

	return result, true
}

func frontmatterExtra(raw map[string]any) map[string]any {
	extra := make(map[string]any, len(raw))
	for k, v := range raw {
		extra[k] = v
	}

	for _, k := range []string{
		"title",
		"status",
		"priority",
		"scheduled",
		"due",
		"dateCreated",
		"dateModified",
		"date_created",
		"date_modified",
		"tags",
	} {
		delete(extra, k)
	}

	return extra
}

func parseWithLayouts(value string, layouts []string) (time.Time, error) {
	var err error
	var t time.Time

	for _, layout := range layouts {
		t, err = time.ParseInLocation(layout, value, time.Local)
		if err == nil {
			return t, nil
		}
	}

	if err == nil {
		err = fmt.Errorf("unsupported datetime format")
	}

	return time.Time{}, fmt.Errorf("%q: %w", value, err)
}
