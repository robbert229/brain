// Package books implements the bookshelf domain (spec 010): parsing Obsidian
// book notes, scanning the Books folder into a shelf catalog, and durably
// creating new review notes. The vault files are the only storage (Principle
// V); everything here is pure functions over bytes and directories so every
// rule is directly testable (Principles I and II).
package books

import (
	"fmt"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Book is one book note projected for the shelf (data-model.md). Only ParseNote
// produces valid values; every field degrades to its zero value rather than
// failing the whole note (FR-004).
type Book struct {
	Title           string
	Review          string
	Author          string
	Cover           string
	Interest        string
	Status          string
	Finished        time.Time
	Rating          float64
	RatingText      string
	Filename        string
	Path            string
	Series          string
	SeriesNumber    float64
	HasSeriesNumber bool
}

// frontmatterDelimiter opens and closes the YAML metadata block.
const frontmatterDelimiter = "---"

// bookMarkerKeys are the frontmatter fields that mark a note as a book review
// (FR-003): index and Waypoint notes carry only tags and must not be listed.
var bookMarkerKeys = []string{"cover", "rating", "status (Media)"}

// imageExtensions are the cover file types accepted for display (research R4).
var imageExtensions = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".webp": true,
	".gif":  true,
}

// IsImageName reports whether name is a single clean path segment naming an
// image file, the same rule covers must satisfy at parse time and at the HTTP
// boundary (FR-005, FR-012).
func IsImageName(name string) bool {
	if name == "" || name != strings.TrimSpace(name) || strings.ContainsAny(name, `/\`) || strings.ContainsRune(name, 0) {
		return false
	}
	if name == "." || name == ".." || name != path.Base(name) {
		return false
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return imageExtensions[strings.ToLower(filepath.Ext(name))]
}

// ParseNote projects one markdown file into a Book, reporting false when the
// note is not a book: no frontmatter block, non-mapping YAML, or none of the
// book marker keys present (FR-003). A true result is always renderable;
// individual malformed fields degrade to absent (FR-004, FR-011).
func ParseNote(filename string, content []byte) (Book, bool) {
	frontmatter, body, ok := splitFrontmatter(string(content))
	if !ok {
		return Book{}, false
	}
	var metadata map[string]any
	if err := yaml.Unmarshal([]byte(frontmatter), &metadata); err != nil || metadata == nil {
		return Book{}, false // scalars and sequences decode to nil maps: not books (research R2)
	}
	if !isBook(metadata) {
		return Book{}, false
	}
	title, review := titleAndReview(body, strings.TrimSuffix(filepath.Base(filename), ".md"))
	rating, ratingText := parseRating(metadata["rating"])
	return Book{
		Title:      title,
		Review:     review,
		Author:     text(metadata["author"]),
		Cover:      cleanCover(metadata["cover"]),
		Interest:   text(metadata["interest"]),
		Status:     text(metadata["status (Media)"]),
		Finished:   parseDate(metadata["finished"]),
		Rating:     rating,
		RatingText: ratingText,
		Filename:   filename,
	}, true
}

// splitFrontmatter returns the YAML block and body, requiring the file to open
// with a delimiter line and carry a closing one.
func splitFrontmatter(content string) (frontmatter, body string, ok bool) {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != frontmatterDelimiter {
		return "", "", false
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == frontmatterDelimiter {
			return strings.Join(lines[1:i], "\n"), strings.Join(lines[i+1:], "\n"), true
		}
	}
	return "", "", false
}

func isBook(metadata map[string]any) bool {
	for _, key := range bookMarkerKeys {
		if _, found := metadata[key]; found {
			return true
		}
	}
	return false
}

// titleAndReview takes the first "# " heading as the title and everything
// after it as the review, falling back to the filename base and the full body
// (data-model.md).
func titleAndReview(body, fallbackTitle string) (title, review string) {
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		if trimmed := strings.TrimSpace(line); strings.HasPrefix(trimmed, "# ") {
			if title = strings.TrimSpace(trimmed[2:]); title != "" {
				return title, strings.TrimSpace(strings.Join(lines[i+1:], "\n"))
			}
		}
	}
	return strings.TrimSpace(fallbackTitle), strings.TrimSpace(body)
}

// text renders scalar metadata as display strings; structured or absent
// values are treated as missing (FR-004).
func text(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case bool:
		return ""
	case float64:
		return strconv.FormatFloat(typed, 'g', -1, 64)
	case int:
		return strconv.Itoa(typed)
	default:
		return ""
	}
}

func cleanCover(value any) string {
	candidate, ok := value.(string)
	if !ok {
		return ""
	}
	candidate = strings.TrimSpace(candidate)
	if !IsImageName(candidate) {
		return ""
	}
	return candidate
}

// parseDate accepts the YAML date coercion (time.Time) and YYYY-MM-DD strings,
// with or without zero padding; anything else sorts undated (data-model.md).
func parseDate(value any) time.Time {
	switch typed := value.(type) {
	case time.Time:
		return typed
	case string:
		if parsed, err := time.Parse("2006-01-02", strings.TrimSpace(typed)); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

// parseRating accepts "n/5" (halves allowed, e.g. "4.5/5") and bare numbers
// within 0–5. Unparseable values yield Rating 0 with the raw text preserved
// for display, and out-of-range or zero-denominator forms degrade to absent.
func parseRating(value any) (float64, string) {
	switch typed := value.(type) {
	case nil:
		return 0, ""
	case float64:
		return bounded(typed, "")
	case int:
		return bounded(float64(typed), "")
	case string:
		raw := strings.TrimSpace(typed)
		if raw == "" {
			return 0, ""
		}
		numerator, denominator := raw, "5"
		if slash := strings.Index(raw, "/"); slash >= 0 {
			numerator, denominator = raw[:slash], raw[slash+1:]
		}
		number, err := strconv.ParseFloat(strings.TrimSpace(numerator), 64)
		if err != nil {
			return 0, raw
		}
		base, err := strconv.ParseFloat(strings.TrimSpace(denominator), 64)
		if err != nil || base <= 0 {
			return 0, raw
		}
		if base != 5 { // normalize any "n/den" scale onto the 5-star display
			number = number / base * 5
		}
		return bounded(number, raw)
	default:
		return 0, ""
	}
}

func bounded(rating float64, raw string) (float64, string) {
	if rating < 0 || rating > 5 {
		if raw == "" {
			return 0, ""
		}
		return 0, raw
	}
	return rating, ""
}

// FormatRating renders the shelf display value for a rating: the normalized
// "n/5" form when parseable, the raw recorded text when not, and nothing when
// absent (data-model.md).
func FormatRating(rating float64, raw string) string {
	if raw != "" {
		return raw
	}
	if rating <= 0 {
		return ""
	}
	return strconv.FormatFloat(rating, 'f', -1, 64) + "/5"
}

// String aids test failure messages only.
func (b Book) String() string {
	return fmt.Sprintf("Book{%s by %s, cover %q, rating %v}", b.Title, b.Author, b.Cover, b.Rating)
}
