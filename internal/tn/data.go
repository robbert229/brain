// Package tn provides TaskNotes format parsing and manipulation utilities.
package tn

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	dateLayout = "2006-01-02"
)

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
	StatusOpen   = "open"
	StatusClosed = "closed"
	StatusNone   = "none"
)

// Common TaskNotes priorities.
const (
	PriorityLow    = "low"
	PriorityNormal = "normal"
	PriorityHigh   = "high"
)

// TaskNote represents one TaskNotes document.
//
// The supported YAML frontmatter fields map to Status, Priority, Scheduled,
// DateCreated, DateModified, and Tags. Body stores the markdown content after
// the frontmatter block.
//
// Title is a derived convenience value populated from the first markdown H1 in
// Body when decoding; it is not written as a dedicated top-level field by this
// package. Extra preserves unknown frontmatter keys so they can round-trip.
type TaskNote struct {
	ID string

	// Status maps to the frontmatter "status" value.
	Status string
	// Priority maps to the frontmatter "priority" value.
	Priority string
	// Scheduled maps to the frontmatter "scheduled" value.
	Scheduled *time.Time
	// DateCreated maps to the frontmatter "dateCreated" value.
	DateCreated *time.Time
	// DateModified maps to the frontmatter "dateModified" value.
	DateModified *time.Time
	// Tags maps to the frontmatter "tags" sequence.
	Tags []string
	// Body contains markdown content after the frontmatter block.
	Body string
	// Title is derived from the first markdown H1 in Body.
	Title string
	// Extra stores unsupported frontmatter fields for round-trip preservation.
	Extra map[string]any
}

type frontmatter struct {
	Status       string   `yaml:"status,omitempty"`
	Priority     string   `yaml:"priority,omitempty"`
	Scheduled    string   `yaml:"scheduled,omitempty"`
	DateCreated  string   `yaml:"dateCreated,omitempty"`
	DateModified string   `yaml:"dateModified,omitempty"`
	Tags         []string `yaml:"tags,omitempty"`
}

// Decode parses a TaskNotes document (YAML frontmatter + markdown body).
func Decode(id string, content string) (*TaskNote, error) {
	note := &TaskNote{Tags: []string{}, Extra: map[string]any{}}
	content = strings.ReplaceAll(content, "\r\n", "\n")

	fmText, body, hasFrontmatter, err := splitFrontmatter(content)
	if err != nil {
		return nil, err
	}

	if hasFrontmatter {
		var fm frontmatter
		if err := yaml.Unmarshal([]byte(fmText), &fm); err != nil {
			return nil, fmt.Errorf("parse frontmatter: %w", err)
		}

		note.Status = fm.Status
		note.Priority = fm.Priority
		note.Tags = append(note.Tags, fm.Tags...)

		if fm.Scheduled != "" {
			t, err := parseWithLayouts(fm.Scheduled, scheduledLayouts)
			if err != nil {
				return nil, fmt.Errorf("parse scheduled: %w", err)
			}

			t.Round(time.Hour * 24)
			note.Scheduled = &t
		}

		if fm.DateCreated != "" {
			t, err := parseWithLayouts(fm.DateCreated, dateTimeLayouts)
			if err != nil {
				return nil, fmt.Errorf("parse dateCreated: %w", err)
			}
			note.DateCreated = &t
		}

		if fm.DateModified != "" {
			t, err := parseWithLayouts(fm.DateModified, dateTimeLayouts)
			if err != nil {
				return nil, fmt.Errorf("parse dateModified: %w", err)
			}
			note.DateModified = &t
		}

		var raw map[string]any
		if err := yaml.Unmarshal([]byte(fmText), &raw); err != nil {
			return nil, fmt.Errorf("parse frontmatter map: %w", err)
		}

		for _, k := range []string{"status", "priority", "scheduled", "dateCreated", "dateModified", "tags"} {
			delete(raw, k)
		}
		note.Extra = raw
	}

	note.Body = body
	note.Title = extractFirstH1(body)
	note.ID = id
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

	body := note.Body
	if body == "" && note.Title != "" {
		body = "# " + note.Title + "\n"
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

	if note.Status != "" {
		appendScalar("status", note.Status)
	}
	if note.Priority != "" {
		appendScalar("priority", note.Priority)
	}
	if note.Scheduled != nil {
		appendScalar("scheduled", note.Scheduled.Format(dateLayout))
	}
	if note.DateCreated != nil {
		appendScalar("dateCreated", note.DateCreated.Format(time.RFC3339Nano))
	}
	if note.DateModified != nil {
		appendScalar("dateModified", note.DateModified.Format(time.RFC3339Nano))
	}
	if len(note.Tags) > 0 {
		mapping.Content = append(mapping.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "tags"},
			stringSliceNode(note.Tags),
		)
	}

	keys := make([]string, 0, len(note.Extra))
	for k := range note.Extra {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		node, err := anyToYAMLNode(note.Extra[k])
		if err != nil {
			return nil, fmt.Errorf("marshal extra field %q: %w", k, err)
		}
		mapping.Content = append(mapping.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: k},
			node,
		)
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
