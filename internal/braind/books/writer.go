package books

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"gopkg.in/yaml.v3"
)

// ReviewDraft is one submitted review form, raw values (data-model.md). The
// web layer trims nothing; validation owns all normalization.
type ReviewDraft struct {
	Title    string
	Author   string
	Rating   string
	Cover    string
	Finished string
	Review   string
}

// ValidationError reports per-field refusals so the form can re-render with
// the submitted values and a message on each offending field (FR-007).
type ValidationError struct {
	Fields map[string]string
}

func (e *ValidationError) Error() string {
	parts := make([]string, 0, len(e.Fields))
	for field, message := range e.Fields {
		parts = append(parts, field+": "+message)
	}
	return "invalid review: " + strings.Join(parts, "; ")
}

// ValidateDraft enforces the data-model.md rules verbatim. maxBytes bounds
// the fully rendered note, not just the prose (FR-010). now bounds the
// finished date against the clock the server sees.
func ValidateDraft(draft ReviewDraft, maxBytes int64, now time.Time) *ValidationError {
	fields := map[string]string{}
	title := strings.TrimSpace(draft.Title)
	switch {
	case title == "":
		fields["title"] = "A title is required."
	case len([]rune(title)) > 200:
		fields["title"] = "The title must be 200 characters or fewer."
	case containsControl(title):
		fields["title"] = "The title must not contain control characters."
	}
	if strings.TrimSpace(draft.Review) == "" {
		fields["review"] = "A review is required."
	}
	if author := strings.TrimSpace(draft.Author); author != "" {
		if len([]rune(author)) > 200 {
			fields["author"] = "The author name must be 200 characters or fewer."
		} else if containsControl(author) {
			fields["author"] = "The author name must not contain control characters."
		}
	}
	if rating := strings.TrimSpace(draft.Rating); rating != "" && !validRating(rating) {
		fields["rating"] = "The rating must be between 0 and 5, as n/5 or a number."
	}
	if cover := strings.TrimSpace(draft.Cover); cover != "" && !IsImageName(cover) {
		fields["cover"] = "The cover must be a single image filename (jpg, jpeg, png, webp, gif)."
	}
	if finished := strings.TrimSpace(draft.Finished); finished != "" {
		date, err := time.Parse("2006-01-02", finished)
		if err != nil {
			fields["finished"] = "The finished date must be formatted YYYY-MM-DD."
		} else if date.After(now) {
			fields["finished"] = "The finished date cannot be in the future."
		}
	}
	if int64(len(renderNote(draft))) > maxBytes {
		fields["review"] = "The finished note is too large; shorten the review."
	}
	if len(fields) == 0 {
		return nil
	}
	return &ValidationError{Fields: fields}
}

func containsControl(s string) bool {
	return strings.ContainsFunc(s, unicode.IsControl)
}

// validRating accepts "n/5" (halves allowed) and bare numbers within 0–5,
// normalizing other denominators onto the five-star scale.
func validRating(raw string) bool {
	numerator, denominator := raw, "5"
	if slash := strings.Index(raw, "/"); slash >= 0 {
		numerator, denominator = raw[:slash], raw[slash+1:]
	}
	number, err := strconv.ParseFloat(strings.TrimSpace(numerator), 64)
	if err != nil {
		return false
	}
	base, err := strconv.ParseFloat(strings.TrimSpace(denominator), 64)
	if err != nil || base <= 0 {
		return false
	}
	if base != 5 {
		number = number / base * 5
	}
	return number >= 0 && number <= 5
}

// CreateReview durably files one new book note in the Books folder root
// (FR-008). "Never overwrites" is a kernel guarantee: the final name is
// published with os.Link, which fails rather than clobber, and colliding
// titles pick slug.md → slug-2.md → slug-3.md. The bytes reach the final name
// only after an fsynced temp write, and every failure path removes the temp
// file (data-model.md state transitions, research R5). It returns the stored
// filename or a *ValidationError / IO error.
func CreateReview(booksDir string, draft ReviewDraft, maxBytes int64, now time.Time) (string, error) {
	if err := ValidateDraft(draft, maxBytes, now); err != nil {
		return "", err
	}
	if err := os.MkdirAll(booksDir, 0o755); err != nil {
		return "", fmt.Errorf("create books folder: %w", err)
	}
	content := renderNote(draft)
	temp, err := os.CreateTemp(booksDir, ".braind-review-*.md")
	if err != nil {
		return "", fmt.Errorf("create temp note: %w", err)
	}
	tempName := temp.Name()
	defer func() {
		if tempName != "" {
			_ = os.Remove(tempName)
		}
	}()
	if _, err := temp.WriteString(content); err != nil {
		temp.Close()
		return "", fmt.Errorf("write temp note: %w", err)
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return "", fmt.Errorf("sync temp note: %w", err)
	}
	if err := temp.Close(); err != nil {
		return "", fmt.Errorf("close temp note: %w", err)
	}
	slug := slugify(draft.Title)
	for attempt := 1; ; attempt++ {
		filename := slug + ".md"
		if attempt > 1 {
			filename = fmt.Sprintf("%s-%d.md", slug, attempt)
		}
		target := filepath.Join(booksDir, filename)
		if err := os.Link(tempName, target); err != nil {
			if os.IsExist(err) {
				continue // title collision: the next suffix is the answer (FR-008)
			}
			return "", fmt.Errorf("publish note: %w", err)
		}
		if err := os.Remove(tempName); err != nil {
			return "", fmt.Errorf("remove temp note: %w", err)
		}
		tempName = ""
		if err := syncDir(booksDir); err != nil {
			return "", err
		}
		return filename, nil
	}
}

// syncDir flushes the folder entry itself so the rename/link survives power
// loss (Data & Storage: durable enough to survive a process restart).
func syncDir(dir string) error {
	handle, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open folder for sync: %w", err)
	}
	defer handle.Close()
	if err := handle.Sync(); err != nil {
		return fmt.Errorf("sync folder: %w", err)
	}
	return nil
}

// renderNote produces the byte-exact corpus convention (data-model.md):
// Obsidian can open the result with metadata intact, no migration (FR-008,
// Principle V). status (Media): Finished is always recorded, matching every
// one of the 130 existing notes; optionals are omitted, never empty.
func renderNote(draft ReviewDraft) string {
	var note strings.Builder
	note.WriteString("---\ntags:\n  - Hobby/Books\n")
	note.WriteString("type: Zettel\n")
	if author := strings.TrimSpace(draft.Author); author != "" {
		fmt.Fprintf(&note, "author: %s\n", author)
	}
	if cover := strings.TrimSpace(draft.Cover); cover != "" {
		fmt.Fprintf(&note, "cover: %s\n", cover)
	}
	note.WriteString("status (Media): Finished\n")
	if finished := strings.TrimSpace(draft.Finished); finished != "" {
		fmt.Fprintf(&note, "finished: %s\n", finished)
	}
	if rating := strings.TrimSpace(draft.Rating); rating != "" {
		fmt.Fprintf(&note, "rating: %s\n", rating)
	}
	note.WriteString("---\n\n")
	fmt.Fprintf(&note, "# %s\n\n", strings.TrimSpace(draft.Title))
	note.WriteString(strings.TrimSpace(draft.Review))
	note.WriteString("\n")
	if cover := strings.TrimSpace(draft.Cover); cover != "" {
		fmt.Fprintf(&note, "\n![[%s]]\n", cover)
	}
	return note.String()
}

// slugify lowercases and joins ASCII alphanumeric words with single dashes;
// a title with nothing sluggable still lands somewhere (research R5).
func slugify(title string) string {
	var slug strings.Builder
	pendingDash := true
	for _, r := range strings.ToLower(title) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			slug.WriteRune(r)
			pendingDash = false
		} else if !pendingDash {
			slug.WriteByte('-')
			pendingDash = true
		}
	}
	cleaned := strings.Trim(slug.String(), "-")
	if cleaned == "" {
		return "book"
	}
	return cleaned
}

// ReadReview resolves one catalog-relative note without following a path out
// of the Books folder and returns its editable fields.
func ReadReview(booksDir, relativePath string, maxBytes int64) (ReviewDraft, error) {
	filename, err := reviewPath(booksDir, relativePath)
	if err != nil {
		return ReviewDraft{}, err
	}
	info, err := os.Lstat(filename)
	if err != nil || !info.Mode().IsRegular() {
		return ReviewDraft{}, fmt.Errorf("book note not found")
	}
	if info.Size() > maxBytes {
		return ReviewDraft{}, fmt.Errorf("book note exceeds size limit")
	}
	content, err := os.ReadFile(filename)
	if err != nil {
		return ReviewDraft{}, fmt.Errorf("read book note: %w", err)
	}
	book, ok := ParseNote(filepath.Base(filename), content)
	if !ok {
		return ReviewDraft{}, fmt.Errorf("not a book note")
	}
	review := book.Review
	if book.Cover != "" {
		review = strings.TrimSpace(strings.ReplaceAll(review, "![["+book.Cover+"]]", ""))
	}
	draft := ReviewDraft{Title: book.Title, Author: book.Author, Rating: FormatRating(book.Rating, book.RatingText), Cover: book.Cover, Review: review}
	if !book.Finished.IsZero() {
		draft.Finished = book.Finished.Format("2006-01-02")
	}
	return draft, nil
}

// UpdateReview atomically updates the selected note in place. Unknown YAML
// fields are retained while the editable book metadata and Markdown body are
// replaced with the submitted values.
func UpdateReview(booksDir, relativePath string, draft ReviewDraft, maxBytes int64, now time.Time) error {
	if invalid := ValidateDraft(draft, maxBytes, now); invalid != nil {
		return invalid
	}
	filename, err := reviewPath(booksDir, relativePath)
	if err != nil {
		return err
	}
	info, err := os.Lstat(filename)
	if err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("book note not found")
	}
	if info.Size() > maxBytes {
		return &ValidationError{Fields: map[string]string{"review": "The existing note is too large to edit."}}
	}
	original, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("read book note: %w", err)
	}
	frontmatter, _, ok := splitFrontmatter(string(original))
	if !ok {
		return fmt.Errorf("invalid book frontmatter")
	}
	metadata := map[string]any{}
	if err := yaml.Unmarshal([]byte(frontmatter), &metadata); err != nil || !isBook(metadata) {
		return fmt.Errorf("invalid book frontmatter")
	}
	setOrDelete := func(key, value string) {
		if value == "" {
			delete(metadata, key)
		} else {
			metadata[key] = value
		}
	}
	setOrDelete("author", strings.TrimSpace(draft.Author))
	setOrDelete("rating", strings.TrimSpace(draft.Rating))
	setOrDelete("cover", strings.TrimSpace(draft.Cover))
	setOrDelete("finished", strings.TrimSpace(draft.Finished))
	yamlBytes, err := yaml.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("encode book frontmatter: %w", err)
	}
	content := "---\n" + string(yamlBytes) + "---\n\n# " + strings.TrimSpace(draft.Title) + "\n\n" + strings.TrimSpace(draft.Review) + "\n"
	if cover := strings.TrimSpace(draft.Cover); cover != "" {
		content += "\n![[" + cover + "]]\n"
	}
	if int64(len(content)) > maxBytes {
		return &ValidationError{Fields: map[string]string{"review": "The finished note is too large; shorten the review."}}
	}
	temp, err := os.CreateTemp(filepath.Dir(filename), ".braind-edit-*.md")
	if err != nil {
		return fmt.Errorf("create temporary book note: %w", err)
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if _, err := temp.WriteString(content); err != nil {
		temp.Close()
		return fmt.Errorf("write temporary book note: %w", err)
	}
	if err := temp.Chmod(info.Mode().Perm()); err != nil {
		temp.Close()
		return fmt.Errorf("set book note permissions: %w", err)
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return fmt.Errorf("sync temporary book note: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temporary book note: %w", err)
	}
	if err := os.Rename(tempName, filename); err != nil {
		return fmt.Errorf("replace book note: %w", err)
	}
	if err := syncDir(filepath.Dir(filename)); err != nil {
		return err
	}
	return nil
}

func reviewPath(booksDir, relativePath string) (string, error) {
	if relativePath == "" || strings.Contains(relativePath, "\\") || !filepath.IsLocal(filepath.FromSlash(relativePath)) || filepath.Ext(relativePath) != ".md" {
		return "", fmt.Errorf("invalid book path")
	}
	root, err := filepath.EvalSymlinks(booksDir)
	if err != nil {
		return "", fmt.Errorf("books folder unavailable")
	}
	filename := filepath.Join(root, filepath.FromSlash(relativePath))
	parent, err := filepath.EvalSymlinks(filepath.Dir(filename))
	if err != nil {
		return "", fmt.Errorf("book folder unavailable")
	}
	rel, err := filepath.Rel(root, parent)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("book path escapes books folder")
	}
	return filepath.Join(parent, filepath.Base(filename)), nil
}
