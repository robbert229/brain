package books

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var testNow = time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)

func TestCreateReviewSerialization(t *testing.T) {
	booksDir := t.TempDir()
	filename, err := CreateReview(booksDir, ReviewDraft{
		Title:    "A Thousand Li 01 - The First Step",
		Author:   "Tao Wong",
		Rating:   "4.5/5",
		Cover:    "book-cover-a-thousand-li-1.jpg",
		Finished: "2025-05-31",
		Review:   "A xinxia/wuxia book. It was relatively short at eight hours, however it managed to pack a lot in.\n\nSecond paragraph.",
	}, 2<<20, testNow)
	if err != nil {
		t.Fatalf("CreateReview: %v", err)
	}
	if filename != "a-thousand-li-01-the-first-step.md" {
		t.Fatalf("filename = %q", filename)
	}
	content, err := os.ReadFile(filepath.Join(booksDir, filename))
	if err != nil {
		t.Fatalf("read note: %v", err)
	}
	want := `---
tags:
  - Hobby/Books
type: Zettel
author: Tao Wong
cover: book-cover-a-thousand-li-1.jpg
status (Media): Finished
finished: 2025-05-31
rating: 4.5/5
---

# A Thousand Li 01 - The First Step

A xinxia/wuxia book. It was relatively short at eight hours, however it managed to pack a lot in.

Second paragraph.

![[book-cover-a-thousand-li-1.jpg]]
`
	if string(content) != want {
		t.Fatalf("note bytes differ:\n got:\n%s\nwant:\n%s", content, want)
	}
	// Round-trip: the generated note parses back as a book with the same fields.
	book, ok := ParseNote(filename, content)
	if !ok || book.Title != "A Thousand Li 01 - The First Step" || book.Rating != 4.5 || book.Cover != "book-cover-a-thousand-li-1.jpg" {
		t.Fatalf("generated note must parse as a book, got %+v ok=%v", book, ok)
	}
}

func TestCreateReviewOptionalFieldsOmitted(t *testing.T) {
	booksDir := t.TempDir()
	filename, err := CreateReview(booksDir, ReviewDraft{Title: "Bare Review", Review: "Just prose."}, 2<<20, testNow)
	if err != nil {
		t.Fatalf("CreateReview: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(booksDir, filename))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	for _, absent := range []string{"author:", "cover:", "finished:", "rating:", "![[", "status (Media): Finished"} {
		if absent == "status (Media): Finished" {
			if !strings.Contains(string(content), absent) {
				t.Errorf("status line must always be recorded (corpus convention)")
			}
			continue
		}
		if strings.Contains(string(content), absent) {
			t.Errorf("empty optional %q must be omitted: %s", absent, content)
		}
	}
}

func TestCreateReviewNeverOverwrites(t *testing.T) {
	booksDir := t.TempDir()
	const original = "precious human-authored content"
	if err := os.WriteFile(filepath.Join(booksDir, "dupe.md"), []byte(original), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Slug of "Dupe" is "dupe": the collision must bump, never clobber (FR-008).
	for i, want := range []string{"dupe-2.md", "dupe-3.md"} {
		filename, err := CreateReview(booksDir, ReviewDraft{Title: "Dupe", Review: fmt.Sprintf("Review %d.", i)}, 2<<20, testNow)
		if err != nil {
			t.Fatalf("CreateReview: %v", err)
		}
		if filename != want {
			t.Fatalf("filename = %q, want %q", filename, want)
		}
	}
	seeded, err := os.ReadFile(filepath.Join(booksDir, "dupe.md"))
	if err != nil || string(seeded) != original {
		t.Fatalf("existing note was modified: %q %v", seeded, err)
	}
	entries, err := os.ReadDir(booksDir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("files = %d, want exactly 3 (no temp files left)", len(entries))
	}
}

func TestCreateReviewRefusesUnsafeInput(t *testing.T) {
	for name, draft := range map[string]ReviewDraft{
		"empty title":        {Title: "   ", Review: "ok"},
		"empty review":       {Title: "T", Review: "  "},
		"title control char": {Title: "Bad\nTitle", Review: "ok"},
		"title too long":     {Title: strings.Repeat("x", 201), Review: "ok"},
		"author too long":    {Title: "T", Review: "ok", Author: strings.Repeat("y", 201)},
		"rating above max":   {Title: "T", Review: "ok", Rating: "6/5"},
		"rating unparseable": {Title: "T", Review: "ok", Rating: "superb"},
		"rating negative":    {Title: "T", Review: "ok", Rating: "-1/5"},
		"cover traversal":    {Title: "T", Review: "ok", Cover: "../secret.jpg"},
		"cover non-image":    {Title: "T", Review: "ok", Cover: "notes.txt"},
		"finished malformed": {Title: "T", Review: "ok", Finished: "31-05-2025"},
		"finished future":    {Title: "T", Review: "ok", Finished: "2026-12-31"},
	} {
		booksDir := t.TempDir()
		_, err := CreateReview(booksDir, draft, 2<<20, testNow)
		var invalid *ValidationError
		if !errors.As(err, &invalid) {
			t.Errorf("%s: err = %v, want ValidationError", name, err)
			continue
		}
		if len(invalid.Fields) == 0 {
			t.Errorf("%s: ValidationError without field messages", name)
		}
		// Refusals write nothing at all: not even a temp file (FR-010).
		if entries, err := os.ReadDir(booksDir); err != nil || len(entries) != 0 {
			t.Errorf("%s: refusal left %v entries", name, entries)
		}
	}
}

func TestCreateReviewEnforcesNoteSizeBound(t *testing.T) {
	booksDir := t.TempDir()
	_, err := CreateReview(booksDir, ReviewDraft{Title: "Big", Review: strings.Repeat("word ", 400)}, 200, testNow)
	var invalid *ValidationError
	if !errors.As(err, &invalid) {
		t.Fatalf("err = %v, want ValidationError for oversize note (FR-010)", err)
	}
	entries, _ := os.ReadDir(booksDir)
	if len(entries) != 0 {
		t.Fatalf("oversize refusal left files: %v", entries)
	}
}

func TestReadAndUpdateReviewPreserveMetadataInPlace(t *testing.T) {
	booksDir := t.TempDir()
	path := filepath.Join(booksDir, "Series", "Book.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir nested folder: %v", err)
	}
	original := "---\ntags:\n  - Hobby/Books\ntype: Zettel\nstatus (Media): Finished\ninterest: 🔥🔥\ncustom: keep this\nauthor: Old Author\ncover: old.jpg\nrating: 4/5\nfinished: 2025-01-02\n---\n\n# Old Title\nOld review.\n\n![[old.jpg]]\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}

	draft, err := ReadReview(booksDir, "Series/Book.md", 2<<20)
	if err != nil {
		t.Fatalf("ReadReview: %v", err)
	}
	if draft.Title != "Old Title" || draft.Author != "Old Author" || draft.Rating != "4/5" || draft.Cover != "old.jpg" || draft.Finished != "2025-01-02" || draft.Review != "Old review." {
		t.Fatalf("ReadReview draft = %+v", draft)
	}

	updated := ReviewDraft{Title: "New Title", Author: "New Author", Rating: "4.5/5", Cover: "new.jpg", Finished: "2025-02-03", Review: "Updated review."}
	if err := UpdateReview(booksDir, "Series/Book.md", updated, 2<<20, testNow); err != nil {
		t.Fatalf("UpdateReview: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read updated note: %v", err)
	}
	for _, want := range []string{"tags:", "Hobby/Books", "type: Zettel", "status (Media): Finished", "interest:", "custom: keep this", "author: New Author", "cover: new.jpg", "rating: 4.5/5", "finished:", "# New Title", "Updated review.", "![[new.jpg]]"} {
		if !strings.Contains(string(content), want) {
			t.Errorf("updated note lacks %q:\n%s", want, content)
		}
	}
	if strings.Contains(string(content), "Old Title") || strings.Contains(string(content), "Old review") || strings.Contains(string(content), "![[old.jpg]]") {
		t.Errorf("old editable content remains:\n%s", content)
	}
	if _, err := os.Stat(filepath.Join(booksDir, "new-title.md")); !os.IsNotExist(err) {
		t.Errorf("edit created a duplicate note: stat error = %v", err)
	}
	book, ok := ParseNote(filepath.Base(path), content)
	if !ok || book.Title != updated.Title || book.Author != updated.Author || book.Cover != updated.Cover || book.Interest != "🔥🔥" || book.Finished.Format("2006-01-02") != updated.Finished {
		t.Fatalf("updated note does not parse as edited book: %+v, ok=%v", book, ok)
	}
}

func TestUpdateReviewRefusesUnsafePaths(t *testing.T) {
	booksDir := t.TempDir()
	outside := t.TempDir()
	outsideNote := filepath.Join(outside, "Book.md")
	if err := os.WriteFile(outsideNote, []byte("precious"), 0o644); err != nil {
		t.Fatalf("write outside note: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(booksDir, "linked")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	valid := ReviewDraft{Title: "New title", Review: "New review"}
	for _, unsafe := range []string{"../Book.md", outsideNote, "linked/Book.md"} {
		if err := UpdateReview(booksDir, unsafe, valid, 2<<20, testNow); err == nil {
			t.Errorf("UpdateReview(%q) unexpectedly succeeded", unsafe)
		}
	}
	content, err := os.ReadFile(outsideNote)
	if err != nil || string(content) != "precious" {
		t.Errorf("unsafe edit modified outside note: %q, %v", content, err)
	}
}

func TestDeleteReviewRemovesOnlyTheBookNote(t *testing.T) {
	booksDir := t.TempDir()
	coversDir := t.TempDir()
	notePath := filepath.Join(booksDir, "Series", "Book.md")
	if err := os.MkdirAll(filepath.Dir(notePath), 0o755); err != nil {
		t.Fatalf("mkdir nested folder: %v", err)
	}
	if err := os.WriteFile(notePath, []byte("---\ntype: Zettel\nstatus (Media): Finished\ncover: shared.jpg\n---\n\n# Book\nReview.\n"), 0o644); err != nil {
		t.Fatalf("write book note: %v", err)
	}
	cover := filepath.Join(coversDir, "shared.jpg")
	if err := os.WriteFile(cover, []byte("shared image bytes"), 0o644); err != nil {
		t.Fatalf("write cover: %v", err)
	}
	unrelated := filepath.Join(booksDir, "Other.md")
	if err := os.WriteFile(unrelated, []byte("---\ntype: Zettel\nstatus (Media): Finished\n---\n# Other\n"), 0o644); err != nil {
		t.Fatalf("write unrelated book: %v", err)
	}

	if err := DeleteReview(booksDir, "Series/Book.md", 2<<20); err != nil {
		t.Fatalf("DeleteReview: %v", err)
	}
	if _, err := os.Stat(notePath); !os.IsNotExist(err) {
		t.Errorf("deleted note still exists: %v", err)
	}
	if got, err := os.ReadFile(cover); err != nil || string(got) != "shared image bytes" {
		t.Errorf("cover was removed or changed: got %q, err %v", got, err)
	}
	if _, err := os.Stat(unrelated); err != nil {
		t.Errorf("unrelated note was removed: %v", err)
	}
}

func TestDeleteReviewRefusesUnsafeTargets(t *testing.T) {
	booksDir := t.TempDir()
	outside := t.TempDir()
	outsideNote := filepath.Join(outside, "Book.md")
	if err := os.WriteFile(outsideNote, []byte("---\nstatus: Finished\n---\n# Outside\n"), 0o644); err != nil {
		t.Fatalf("write outside note: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(booksDir, "linked")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := os.WriteFile(filepath.Join(booksDir, "Plain.md"), []byte("# Not a book note\n"), 0o644); err != nil {
		t.Fatalf("write non-book note: %v", err)
	}
	if err := os.Mkdir(filepath.Join(booksDir, "folder.md"), 0o755); err != nil {
		t.Fatalf("write directory target: %v", err)
	}
	for _, unsafe := range []string{"../Book.md", outsideNote, "linked/Book.md", "Plain.md", "missing.md", "folder.md"} {
		if err := DeleteReview(booksDir, unsafe, 2<<20); err == nil {
			t.Errorf("DeleteReview(%q) unexpectedly succeeded", unsafe)
		}
	}
	if got, err := os.ReadFile(outsideNote); err != nil || !strings.Contains(string(got), "Outside") {
		t.Errorf("outside note was modified: got %q, err %v", got, err)
	}
}

func TestSlugify(t *testing.T) {
	for input, want := range map[string]string{
		"A Thousand Li 01 - The First Step":                    "a-thousand-li-01-the-first-step",
		"  Dungeon Crawler Carl 6 - Eye of the Bedlam Bride  ": "dungeon-crawler-carl-6-eye-of-the-bedlam-bride",
		"***":         "book",
		"!!!":         "book",
		"Café Bücher": "caf-b-cher",
	} {
		if got := slugify(input); got != want {
			t.Errorf("slugify(%q) = %q, want %q", input, got, want)
		}
	}
}
