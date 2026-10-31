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
	Title        string
	Author       string
	Rating       string
	Cover        string
	Finished     string
	Review       string
	Series       string
	SeriesNumber string
	SeriesSet    bool
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
	series := strings.TrimSpace(draft.Series)
	seriesNumber := strings.TrimSpace(draft.SeriesNumber)
	if len([]rune(series)) > 100 || len(series) > 200 || containsControl(series) || strings.ContainsAny(series, `/\\`) || series == "." || series == ".." {
		fields["series"] = "The series must be a folder name without slashes or control characters (100 characters or fewer)."
	}
	if seriesNumber != "" {
		if series == "" {
			fields["series_number"] = "A series is required when a volume number is provided."
		} else if !validSeriesNumber(seriesNumber) {
			fields["series_number"] = "The volume number must be a non-negative integer or decimal, such as 8 or 8.5."
		}
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

func validSeriesNumber(raw string) bool {
	if len(raw) > 24 || strings.Count(raw, ".") > 1 {
		return false
	}
	for _, r := range raw {
		if (r < '0' || r > '9') && r != '.' {
			return false
		}
	}
	if raw == "" || strings.HasPrefix(raw, ".") || strings.HasSuffix(raw, ".") {
		return false
	}
	number, err := strconv.ParseFloat(raw, 64)
	return err == nil && number >= 0 && !strings.Contains(strings.ToLower(raw), "nan")
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

// CreateReview durably files one new book note in the Books root or its
// assigned series folder (FR-008, FR-026). "Never overwrites" is a kernel guarantee: the final name is
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
	targetDir, err := seriesDirectory(booksDir, strings.TrimSpace(draft.Series))
	if err != nil {
		return "", err
	}
	filename, err := writeNewNote(targetDir, generatedFilenameBase(draft), renderNote(draft))
	if err != nil {
		return "", err
	}
	if series := strings.TrimSpace(draft.Series); series != "" {
		filename = filepath.ToSlash(filepath.Join(series, filename))
	}
	return filename, nil
}

func writeNewNote(dir, base, content string) (string, error) {
	temp, err := os.CreateTemp(dir, ".braind-review-*.md")
	if err != nil {
		return "", fmt.Errorf("create temp note: %w", err)
	}
	tempName := temp.Name()
	defer func() { _ = os.Remove(tempName) }()
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
	for attempt := 1; ; attempt++ {
		filename := base + ".md"
		if attempt > 1 {
			filename = fmt.Sprintf("%s-%d.md", base, attempt)
		}
		if err := os.Link(tempName, filepath.Join(dir, filename)); err != nil {
			if os.IsExist(err) {
				continue
			}
			return "", fmt.Errorf("publish note: %w", err)
		}
		if err := os.Remove(tempName); err != nil {
			return "", fmt.Errorf("remove temp note: %w", err)
		}
		if err := syncDir(dir); err != nil {
			return "", err
		}
		return filename, nil
	}
}

func seriesDirectory(booksDir, series string) (string, error) {
	root, err := filepath.EvalSymlinks(booksDir)
	if err != nil {
		return "", fmt.Errorf("books folder unavailable: %w", err)
	}
	if series == "" {
		return root, nil
	}
	dir := filepath.Join(root, series)
	info, err := os.Lstat(dir)
	if os.IsNotExist(err) {
		if err := os.Mkdir(dir, 0o755); err != nil && !os.IsExist(err) {
			return "", fmt.Errorf("create series folder: %w", err)
		}
		info, err = os.Lstat(dir)
	}
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", fmt.Errorf("series folder unavailable")
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", fmt.Errorf("series folder unavailable: %w", err)
	}
	rel, err := filepath.Rel(root, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("series folder escapes books folder")
	}
	return resolved, nil
}

func generatedFilenameBase(draft ReviewDraft) string {
	slug := slugify(draft.Title)
	series := strings.TrimSpace(draft.Series)
	if series == "" {
		return slug
	}
	position := strings.TrimSpace(draft.SeriesNumber)
	prefix := series + " "
	if position != "" {
		position = canonicalSeriesNumber(position)
		prefix += position + " - "
	} else {
		prefix += "- "
	}
	maxSlugBytes := 240 - len(prefix)
	if maxSlugBytes < 1 {
		maxSlugBytes = 1
	}
	if len(slug) > maxSlugBytes {
		slug = strings.TrimRight(slug[:maxSlugBytes], "-")
	}
	return prefix + slug
}

func canonicalSeriesNumber(raw string) string {
	number, _ := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	value := strconv.FormatFloat(number, 'f', -1, 64)
	if dot := strings.IndexByte(value, '.'); dot >= 0 {
		integer := value[:dot]
		if len(integer) < 2 {
			integer = "0" + integer
		}
		return integer + value[dot:]
	}
	if len(value) < 2 {
		return "0" + value
	}
	return value
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
	if filepath.Dir(filepath.FromSlash(relativePath)) != "." {
		draft.Series = strings.Split(filepath.ToSlash(relativePath), "/")[0]
		if number, ok := parseSeriesNumber(filepath.Base(relativePath), draft.Series); ok {
			draft.SeriesNumber = strconv.FormatFloat(number, 'f', -1, 64)
		}
	}
	draft.SeriesSet = true
	if !book.Finished.IsZero() {
		draft.Finished = book.Finished.Format("2006-01-02")
	}
	return draft, nil
}

// UpdateReview atomically updates the selected note. Unknown YAML fields are
// retained while the editable book metadata and Markdown body are replaced;
// an explicitly changed series assignment moves the note to its new path.
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
	series, oldNumber, hadNumber := seriesAssignment(relativePath)
	unchangedAssignment := !draft.SeriesSet || (strings.TrimSpace(draft.Series) == series && sameSeriesNumber(strings.TrimSpace(draft.SeriesNumber), oldNumber, hadNumber))
	var targetDir string
	var targetBase string
	if unchangedAssignment {
		targetDir = filepath.Dir(filename)
		targetBase = strings.TrimSuffix(filepath.Base(filename), ".md")
	} else {
		targetDir, err = seriesDirectory(booksDir, strings.TrimSpace(draft.Series))
		if err != nil {
			return err
		}
		targetBase = generatedFilenameBase(draft)
	}
	target := filepath.Join(targetDir, targetBase+".md")
	if err := replaceReviewNote(filename, target, targetDir, targetBase, content, info.Mode().Perm()); err != nil {
		return err
	}
	return nil
}

func replaceReviewNote(filename, target string, targetDir, targetBase, content string, mode os.FileMode) error {
	temp, err := os.CreateTemp(targetDir, ".braind-edit-*.md")
	if err != nil {
		return fmt.Errorf("create temporary book note: %w", err)
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if _, err := temp.WriteString(content); err != nil {
		temp.Close()
		return fmt.Errorf("write temporary book note: %w", err)
	}
	if err := temp.Chmod(mode); err != nil {
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
	if filepath.Clean(target) == filepath.Clean(filename) {
		if err := os.Rename(tempName, filename); err != nil {
			return fmt.Errorf("replace book note: %w", err)
		}
		return syncDir(filepath.Dir(filename))
	}
	for attempt := 1; ; attempt++ {
		candidate := target
		if attempt > 1 {
			suffix := fmt.Sprintf("-%d.md", attempt)
			candidate = filepath.Join(targetDir, targetBase+suffix)
		}
		if err := os.Link(tempName, candidate); err != nil {
			if os.IsExist(err) {
				continue
			}
			return fmt.Errorf("publish updated note: %w", err)
		}
		if err := syncDir(targetDir); err != nil {
			_ = os.Remove(candidate)
			return err
		}
		if err := os.Remove(filename); err != nil {
			_ = os.Remove(candidate)
			return fmt.Errorf("remove old book note: %w", err)
		}
		if err := syncDir(filepath.Dir(filename)); err != nil {
			return err
		}
		return nil
	}
}

func seriesAssignment(relativePath string) (string, string, bool) {
	clean := filepath.FromSlash(relativePath)
	if filepath.Dir(clean) == "." {
		return "", "", false
	}
	series := strings.Split(filepath.ToSlash(clean), "/")[0]
	number, ok := parseSeriesNumber(filepath.Base(clean), series)
	if !ok {
		return series, "", false
	}
	return series, strconv.FormatFloat(number, 'f', -1, 64), true
}

func sameSeriesNumber(submitted, existing string, existingSet bool) bool {
	if !existingSet {
		return submitted == ""
	}
	if !validSeriesNumber(submitted) {
		return false
	}
	left, leftErr := strconv.ParseFloat(submitted, 64)
	right, rightErr := strconv.ParseFloat(existing, 64)
	return leftErr == nil && rightErr == nil && left == right
}

// DeleteReview removes one bounded, valid book note beneath booksDir. It does
// not remove cover attachments because another note may reference them.
func DeleteReview(booksDir, relativePath string, maxBytes int64) error {
	filename, err := reviewPath(booksDir, relativePath)
	if err != nil {
		return err
	}
	info, err := os.Lstat(filename)
	if err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("book note not found")
	}
	if info.Size() > maxBytes {
		return fmt.Errorf("book note exceeds size limit")
	}
	content, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("read book note: %w", err)
	}
	if _, ok := ParseNote(filepath.Base(filename), content); !ok {
		return fmt.Errorf("not a book note")
	}
	if err := os.Remove(filename); err != nil {
		return fmt.Errorf("remove book note: %w", err)
	}
	if err := syncDir(filepath.Dir(filename)); err != nil {
		return fmt.Errorf("sync book folder after deletion: %w", err)
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
