package tnmodel

import (
	"fmt"
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

		normalized := normalizeFrontmatterFields(raw)
		fmBytes, err := yaml.Marshal(normalized)
		if err != nil {
			return nil, fmt.Errorf("marshal normalized frontmatter: %w", err)
		}

		if err := yaml.Unmarshal(fmBytes, &note.Frontmatter); err != nil {
			return nil, fmt.Errorf("parse frontmatter: %w", err)
		}
		if err := validateFrontmatterDates(note.Frontmatter); err != nil {
			return nil, err
		}

		note.Frontmatter.AdditionalProperties = frontmatterExtra(normalized)
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

	fm, err := frontmatterMap(note.Frontmatter)
	if err != nil {
		return "", err
	}

	fmBytes, err := yaml.Marshal(fm)
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

func frontmatterMap(fm TaskFrontmatter) (map[string]any, error) {
	extra, _ := fm.AdditionalProperties.(map[string]any)
	fm.AdditionalProperties = nil

	b, err := yaml.Marshal(fm)
	if err != nil {
		return nil, fmt.Errorf("marshal frontmatter: %w", err)
	}

	var fields map[string]any
	if err := yaml.Unmarshal(b, &fields); err != nil {
		return nil, fmt.Errorf("parse generated frontmatter: %w", err)
	}

	deleteEmptyGeneratedFields(fields, fm)
	for k, v := range extra {
		fields[k] = v
	}

	return fields, nil
}

func stringPtr(value string) *string {
	return &value
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

func normalizeFrontmatterFields(raw map[string]any) map[string]any {
	normalized := make(map[string]any, len(raw))
	for k, v := range raw {
		normalized[k] = normalizeYAMLScalar(v)
	}

	copyLegacyField(normalized, "dateCreated", "date_created")
	copyLegacyField(normalized, "dateModified", "date_modified")
	normalizeDateTimeField(normalized, "date_created")
	normalizeDateTimeField(normalized, "date_modified")
	return normalized
}

func normalizeDateTimeField(values map[string]any, key string) {
	value, ok := values[key].(string)
	if !ok {
		return
	}

	t, err := parseWithLayouts(value, dateTimeLayouts)
	if err != nil {
		return
	}

	values[key] = t.Format(time.RFC3339Nano)
}

func copyLegacyField(values map[string]any, legacy string, canonical string) {
	if _, ok := values[canonical]; ok {
		delete(values, legacy)
		return
	}

	if value, ok := values[legacy]; ok {
		values[canonical] = value
		delete(values, legacy)
	}
}

func normalizeYAMLScalar(value any) any {
	t, ok := value.(time.Time)
	if !ok {
		return value
	}

	if t.Hour() == 0 && t.Minute() == 0 && t.Second() == 0 && t.Nanosecond() == 0 {
		return t.Format(dateLayout)
	}

	return t.Format(time.RFC3339Nano)
}

func validateFrontmatterDates(fm TaskFrontmatter) error {
	if fm.Scheduled != nil {
		if _, err := parseWithLayouts(string(*fm.Scheduled), scheduledLayouts); err != nil {
			return fmt.Errorf("parse scheduled: %w", err)
		}
	}
	if fm.Due != nil {
		if _, err := parseWithLayouts(string(*fm.Due), scheduledLayouts); err != nil {
			return fmt.Errorf("parse due: %w", err)
		}
	}

	return nil
}

func deleteEmptyGeneratedFields(fields map[string]any, fm TaskFrontmatter) {
	delete(fields, "additionalproperties")
	if fm.Title == "" {
		delete(fields, "title")
	}
	if fm.Status == "" {
		delete(fields, "status")
	}
	if fm.DateCreated.IsZero() {
		delete(fields, "date_created")
	}
	if fm.DateModified.IsZero() {
		delete(fields, "date_modified")
	}
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
