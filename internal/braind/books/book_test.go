package books

import (
	"strings"
	"testing"
	"time"
)

// corpusNote is the real-world book note shape from spec 010's input example;
// every parse rule is pinned against this layout, not a synthetic one.
const corpusNote = `---
tags:
  - Hobby/Books
type: Zettel
author: Tao Wong
cover: book-cover-a-thousand-li-1.jpg
interest: 🔥🔥
status (Media): Finished
finished: 2025-05-31
rating: 4.5/5
---

# A Thousand Li 01 - The First Step
A xinxia/wuxia book. It was relatively short at eight hours, however it
managed to pack a lot in without it being rushed.

![[book-cover-a-thousand-li-1.jpg]]
`

func TestParseNoteCorpusShape(t *testing.T) {
	book, ok := ParseNote("A Thousand Li 1.md", []byte(corpusNote))
	if !ok {
		t.Fatal("corpus note must parse as a book")
	}
	if book.Title != "A Thousand Li 01 - The First Step" {
		t.Errorf("Title = %q", book.Title)
	}
	if book.Author != "Tao Wong" {
		t.Errorf("Author = %q", book.Author)
	}
	if book.Cover != "book-cover-a-thousand-li-1.jpg" {
		t.Errorf("Cover = %q", book.Cover)
	}
	if book.Interest != "🔥🔥" {
		t.Errorf("Interest = %q", book.Interest)
	}
	if book.Status != "Finished" {
		t.Errorf("Status = %q", book.Status)
	}
	if want := (time.Date(2025, 5, 31, 0, 0, 0, 0, time.UTC)); !book.Finished.Equal(want) {
		t.Errorf("Finished = %v, want %v", book.Finished, want)
	}
	if book.Rating != 4.5 || book.RatingText != "" {
		t.Errorf("Rating = %v/%q, want 4.5 clean", book.Rating, book.RatingText)
	}
	if !strings.HasPrefix(book.Review, "A xinxia/wuxia book.") || !strings.Contains(book.Review, "![[book-cover-a-thousand-li-1.jpg]]") {
		t.Errorf("Review = %q", book.Review)
	}
	if book.Filename != "A Thousand Li 1.md" {
		t.Errorf("Filename = %q", book.Filename)
	}
}

func TestParseNoteNonBooks(t *testing.T) {
	for name, content := range map[string]string{
		"waypoint index": "---\ntags:\n- Hobby/Books\n---\n# Books\n%% Begin Waypoint %%\n",
		"no frontmatter": "# Just a thought\nno metadata here\n",
		"unclosed":       "---\ntags:\n  - Hobby/Books\ncover: x.jpg\n",
		"scalar yaml":    "---\njust a string\n---\nbody\n",
		"sequence yaml":  "---\n- a\n- b\n---\nbody\n",
		"empty mapping":  "---\n---\n# Title\n",
	} {
		if _, ok := ParseNote("note.md", []byte(content)); ok {
			t.Errorf("%s: parsed as a book, want skipped (FR-003)", name)
		}
	}
}

func TestParseNoteMarkerKeyPresence(t *testing.T) {
	for _, marker := range []string{"cover: a.jpg", "rating: 4/5", `status (Media): Finished`} {
		content := "---\n" + marker + "\n---\n# T\nreview\n"
		if _, ok := ParseNote("t.md", []byte(content)); !ok {
			t.Errorf("marker %q must make a book", marker)
		}
	}
}

func TestParseNoteFieldLeniency(t *testing.T) {
	tests := []struct {
		name     string
		note     string
		filename string
		check    func(*testing.T, Book)
	}{
		{
			name:     "no heading falls back to filename and full body",
			note:     "---\nrating: 3/5\n---\nSolo review paragraph.\n",
			filename: "Untitled Note.md",
			check: func(t *testing.T, b Book) {
				if b.Title != "Untitled Note" {
					t.Errorf("Title = %q, want filename base", b.Title)
				}
				if b.Review != "Solo review paragraph." {
					t.Errorf("Review = %q", b.Review)
				}
			},
		},
		{
			name: "bare numeric rating",
			note: "---\nrating: 4.5\n---\n# T\nr\n",
			check: func(t *testing.T, b Book) {
				if b.Rating != 4.5 || b.RatingText != "" {
					t.Errorf("Rating = %v/%q", b.Rating, b.RatingText)
				}
			},
		},
		{
			name: "out-of-range rating degrades to raw text",
			note: "---\nrating: 9/5\n---\n# T\nr\n",
			check: func(t *testing.T, b Book) {
				if b.Rating != 0 || b.RatingText != "9/5" {
					t.Errorf("Rating = %v/%q, want 0 with raw text", b.Rating, b.RatingText)
				}
			},
		},
		{
			name: "unparseable rating keeps raw text",
			note: "---\nrating: superb\n---\n# T\nr\n",
			check: func(t *testing.T, b Book) {
				if b.Rating != 0 || b.RatingText != "superb" {
					t.Errorf("Rating = %v/%q", b.Rating, b.RatingText)
				}
			},
		},
		{
			name: "other-scale rating normalizes onto five",
			note: "---\nrating: 8/10\n---\n# T\nr\n",
			check: func(t *testing.T, b Book) {
				if b.Rating != 4 || b.RatingText != "" {
					t.Errorf("Rating = %v/%q, want 4", b.Rating, b.RatingText)
				}
			},
		},
		{
			name: "unpadded date accepted",
			note: "---\nfinished: 2025-5-3\nrating: 5/5\n---\n# T\nr\n",
			check: func(t *testing.T, b Book) {
				if b.Finished.IsZero() {
					t.Error("unpadded date should parse")
				}
				if b.Rating != 5 {
					t.Errorf("Rating = %v, want 5", b.Rating)
				}
			},
		},
		{
			name: "invalid date sorts undated",
			note: "---\nfinished: whenever\nrating: 5/5\n---\n# T\nr\n",
			check: func(t *testing.T, b Book) {
				if !b.Finished.IsZero() {
					t.Errorf("Finished = %v, want zero", b.Finished)
				}
			},
		},
		{
			name: "unsafe cover treated as absent",
			note: "---\ncover: ../../etc/passwd.jpg\nrating: 5/5\n---\n# T\nr\n",
			check: func(t *testing.T, b Book) {
				if b.Cover != "" {
					t.Errorf("Cover = %q, want absent (FR-012)", b.Cover)
				}
			},
		},
		{
			name: "nested-path cover treated as absent",
			note: "---\ncover: attachments/x.png\nrating: 5/5\n---\n# T\nr\n",
			check: func(t *testing.T, b Book) {
				if b.Cover != "" {
					t.Errorf("Cover = %q, want absent", b.Cover)
				}
			},
		},
		{
			name: "non-image cover treated as absent",
			note: "---\ncover: notes.pdf\nrating: 5/5\n---\n# T\nr\n",
			check: func(t *testing.T, b Book) {
				if b.Cover != "" {
					t.Errorf("Cover = %q, want absent", b.Cover)
				}
			},
		},
		{
			name: "structured author treated as absent",
			note: "---\nauthor:\n  name: nested\nrating: 5/5\n---\n# T\nr\n",
			check: func(t *testing.T, b Book) {
				if b.Author != "" {
					t.Errorf("Author = %q, want absent", b.Author)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			filename := test.filename
			if filename == "" {
				filename = "note.md"
			}
			book, ok := ParseNote(filename, []byte(test.note))
			if !ok {
				t.Fatal("note carries a marker key, want book")
			}
			test.check(t, book)
		})
	}
}

func TestFormatRating(t *testing.T) {
	tests := []struct {
		rating float64
		raw    string
		want   string
	}{
		{4.5, "", "4.5/5"},
		{5, "", "5/5"},
		{3.5, "", "3.5/5"},
		{4, "", "4/5"},
		{0, "superb", "superb"},
		{0, "", ""},
	}
	for _, test := range tests {
		if got := FormatRating(test.rating, test.raw); got != test.want {
			t.Errorf("FormatRating(%v, %q) = %q, want %q", test.rating, test.raw, got, test.want)
		}
	}
}

func TestIsImageName(t *testing.T) {
	for _, name := range []string{"cover.jpg", "cover.jpeg", "a.png", "b.WEBP", "c.gif", "with space.jpg", "dashes-and_underscores.jpg"} {
		if !IsImageName(name) {
			t.Errorf("IsImageName(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"", ".", "..", "x.jpg/", "dir/x.jpg", `dir\x.jpg`, "x.svg", "x.txt", "../x.jpg", "/abs/x.jpg", "x.jpg\x00", "x\ny.jpg", " lead.jpg"} {
		if IsImageName(name) {
			t.Errorf("IsImageName(%q) = true, want false (FR-012)", name)
		}
	}
}
