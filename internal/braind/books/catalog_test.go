package books

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeNote places a markdown file (creating parent folders) in the fixture
// Books folder.
func writeNote(t *testing.T, booksDir, rel, content string) {
	t.Helper()
	path := filepath.Join(booksDir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}
}

func bookNote(title, cover, finished, rating string) string {
	var b strings.Builder
	b.WriteString("---\ntags:\n  - Hobby/Books\ntype: Zettel\n")
	if cover != "" {
		fmt.Fprintf(&b, "cover: %s\n", cover)
	}
	b.WriteString("status (Media): Finished\n")
	if finished != "" {
		fmt.Fprintf(&b, "finished: %s\n", finished)
	}
	if rating != "" {
		fmt.Fprintf(&b, "rating: %s\n", rating)
	}
	fmt.Fprintf(&b, "---\n\n# %s\nReview of %s.\n", title, title)
	return b.String()
}

func validImageBytes(t *testing.T, ext string) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.White)
	var encoded bytes.Buffer
	var err error
	switch ext {
	case ".jpg", ".jpeg":
		err = jpeg.Encode(&encoded, img, nil)
	case ".png":
		err = png.Encode(&encoded, img)
	case ".gif":
		err = gif.Encode(&encoded, img, nil)
	case ".webp":
		return []byte("RIFF\x10\x00\x00\x00WEBPVP8 \x04\x00\x00\x00DATA")
	default:
		t.Fatalf("unsupported fixture extension %q", ext)
	}
	if err != nil {
		t.Fatalf("encode %s fixture: %v", ext, err)
	}
	return encoded.Bytes()
}

// fixtureShelf mirrors the live layout: nested series folders, Waypoint index
// notes, a coverless book, and a malformed note (quickstart.md).
func fixtureShelf(t *testing.T) (booksDir, coversDir string) {
	t.Helper()
	booksDir, coversDir = t.TempDir(), t.TempDir()
	writeNote(t, booksDir, "Dungeon Crawler Carl 6.md", bookNote("Dungeon Crawler Carl 6", "dcc6.jpg", "2025-03-12", "5/5"))
	writeNote(t, booksDir, "Level One God/Level One God 1.md", bookNote("Level One God 1", "log1.jpg", "2024-11-02", "4/5"))
	writeNote(t, booksDir, "Level One God/Level One God 2.md", bookNote("Level One God 2", "", "2024-12-25", "3.5/5"))
	writeNote(t, booksDir, "Undated/Undated Find.md", bookNote("Undated Find", "", "", "4/5"))
	writeNote(t, booksDir, "Books.md", "---\ntags:\n  - Hobby/Books\n---\n# Books\n%% Begin Waypoint %%\n")
	writeNote(t, booksDir, "Level One God/Level One God.md", "---\ntags:\n- Hobby/Books\n---\n# Level One God\n%% Begin Waypoint %%\n")
	writeNote(t, booksDir, "Broken.md", "---\ncover: broken.jpg\n\tbroken: true\n---\n# Broken\n")
	if err := os.WriteFile(filepath.Join(coversDir, "dcc6.jpg"), validImageBytes(t, ".jpg"), 0o644); err != nil {
		t.Fatalf("write cover: %v", err)
	}
	if err := os.WriteFile(filepath.Join(coversDir, "log1.jpg"), nil, 0o644); err != nil {
		t.Fatalf("write empty cover: %v", err)
	}
	return booksDir, coversDir
}

func TestScanFindsBooksAtAnyDepth(t *testing.T) {
	booksDir, coversDir := fixtureShelf(t)
	catalog, err := Scan(booksDir, coversDir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	var titles []string
	for _, entry := range catalog.Entries {
		titles = append(titles, entry.Title)
	}
	want := []string{"Dungeon Crawler Carl 6", "Level One God 2", "Level One God 1", "Undated Find"}
	if strings.Join(titles, "|") != strings.Join(want, "|") {
		t.Fatalf("titles = %v, want %v (FR-002, FR-006)", titles, want)
	}
	if catalog.Truncated {
		t.Error("small shelf must not report truncation")
	}
}

func TestScanExcludesNonBooksAndCountsSkips(t *testing.T) {
	booksDir, coversDir := fixtureShelf(t)
	catalog, err := Scan(booksDir, coversDir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	for _, entry := range catalog.Entries {
		if entry.Title == "Books" || entry.Title == "Level One God" {
			t.Fatalf("index note listed as a book: %v (FR-003)", entry.Title)
		}
	}
	// Books.md, Level One God.md (Waypoint) and Broken.md are the three skips.
	if catalog.Skipped != 3 {
		t.Fatalf("Skipped = %d, want 3 (FR-011)", catalog.Skipped)
	}
}

func TestScanCoverResolution(t *testing.T) {
	booksDir, coversDir := fixtureShelf(t)
	catalog, err := Scan(booksDir, coversDir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	found := map[string]bool{}
	for _, entry := range catalog.Entries {
		found[entry.Title] = entry.CoverFound
	}
	if !found["Dungeon Crawler Carl 6"] {
		t.Error("existing cover must resolve (SC-003)")
	}
	if found["Level One God 1"] {
		t.Error("empty file is not a cover")
	}
	if found["Level One God 2"] || found["Undated Find"] {
		t.Error("notes without covers must not claim one")
	}
}

func TestCoverFoundRequiresReadableSupportedImage(t *testing.T) {
	coversDir := t.TempDir()
	for _, ext := range []string{".jpg", ".png", ".gif", ".webp"} {
		if err := os.WriteFile(filepath.Join(coversDir, "valid"+ext), validImageBytes(t, ext), 0o644); err != nil {
			t.Fatalf("write valid %s cover: %v", ext, err)
		}
	}
	if err := os.WriteFile(filepath.Join(coversDir, "corrupt.jpg"), []byte("not an image"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(coversDir, "empty.png"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(coversDir, "unreadable.jpg"), validImageBytes(t, ".jpg"), 0o000); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.jpg")
	if err := os.WriteFile(outside, validImageBytes(t, ".jpg"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(coversDir, "outside.jpg")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	for _, ext := range []string{".jpg", ".png", ".gif", ".webp"} {
		if !coverFound(coversDir, "valid"+ext) {
			t.Errorf("valid supported image %s was not recognized", ext)
		}
	}
	for _, name := range []string{"missing.jpg", "corrupt.jpg", "empty.png", "unreadable.jpg", "outside.jpg", "notes.txt"} {
		if coverFound(coversDir, name) {
			t.Errorf("unavailable image %q was recognized as a cover", name)
		}
	}
}

func TestScanMissingFolderIsEmptyState(t *testing.T) {
	catalog, err := Scan(filepath.Join(t.TempDir(), "does-not-exist"), t.TempDir())
	if err != nil {
		t.Fatalf("missing Books folder must not error: %v", err)
	}
	if len(catalog.Entries) != 0 || catalog.Skipped != 0 {
		t.Fatalf("catalog = %+v, want empty", catalog)
	}
}

func TestScanTruncatesAtVisitBudget(t *testing.T) {
	booksDir, coversDir := t.TempDir(), t.TempDir()
	for i := 0; i <= maxVisitedFiles; i++ {
		writeNote(t, booksDir, fmt.Sprintf("book-%05d.md", i), bookNote(fmt.Sprintf("Book %05d", i), "", "2025-01-01", ""))
	}
	catalog, err := Scan(booksDir, coversDir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if !catalog.Truncated {
		t.Fatal("exceeding the visit budget must set Truncated (research R6)")
	}
	if len(catalog.Entries) > maxVisitedFiles {
		t.Fatalf("listed %d entries, want at most the %d budget", len(catalog.Entries), maxVisitedFiles)
	}
}

func TestSortOrder(t *testing.T) {
	entries := []Entry{
		{Book: Book{Title: "Undated One"}},
		{Book: Book{Title: "alpha", Finished: mustDate(t, "2025-01-01")}},
		{Book: Book{Title: "Beta", Finished: mustDate(t, "2025-01-01")}},
		{Book: Book{Title: "Newer", Finished: mustDate(t, "2026-01-01")}},
		{Book: Book{Title: "Second Undated"}},
	}
	Sort(entries)
	var got []string
	for _, e := range entries {
		got = append(got, e.Title)
	}
	want := "Newer|alpha|Beta|Second Undated|Undated One"
	if strings.Join(got, "|") != want {
		t.Fatalf("order = %v, want %q (FR-006)", got, want)
	}
}

func mustDate(t *testing.T, s string) time.Time {
	t.Helper()
	when, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatalf("parse date: %v", err)
	}
	return when
}
