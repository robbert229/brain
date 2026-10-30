package books

import (
	"encoding/binary"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	// maxVisitedFiles bounds one scan so a pathological tree cannot pin a
	// request (research R6); reaching it sets Truncated instead of failing.
	maxVisitedFiles = 2000
	// maxNoteBytes bounds a single note read; notes past it are skipped.
	maxNoteBytes = 2 << 20
)

// Entry is one shelf card's data: the parsed note plus whether its cover
// resolved to a readable image (the CoverRef of data-model.md — decided at
// scan time so the template never emits a broken <img>, SC-003).
type Entry struct {
	Book
	CoverFound bool
}

// Catalog is the derived shelf: recomputed per request from the vault and
// never persisted (Principle V, research R6).
type Catalog struct {
	Entries   []Entry
	Skipped   int
	Truncated bool
}

// Scan recursively projects every book note under booksDir (FR-002). A
// missing or unstatable folder is the "no books yet" state, not an error.
// Non-book, unreadable, and oversized markdown notes increment Skipped while
// every other note still lists (FR-011). Cover existence is checked against
// coversDir with one Stat per cover (research R4). Entries arrive sorted per
// Sort (FR-006).
func Scan(booksDir, coversDir string) (Catalog, error) {
	var catalog Catalog
	if info, err := os.Stat(booksDir); err != nil || !info.IsDir() {
		return catalog, nil
	}
	visited := 0
	walkErr := filepath.WalkDir(booksDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && isMarkdown(d.Name()) {
				catalog.Skipped++
			}
			return nil // never abort the shelf for one unreadable path (FR-011)
		}
		if d.IsDir() {
			return nil
		}
		visited++
		if visited > maxVisitedFiles {
			catalog.Truncated = true
			return fs.SkipAll
		}
		if !isMarkdown(d.Name()) {
			return nil
		}
		info, statErr := d.Info()
		if statErr != nil || info.Size() > maxNoteBytes {
			catalog.Skipped++
			return nil
		}
		content, readErr := os.ReadFile(p)
		if readErr != nil {
			catalog.Skipped++
			return nil
		}
		book, ok := ParseNote(d.Name(), content)
		if !ok {
			catalog.Skipped++
			return nil
		}
		catalog.Entries = append(catalog.Entries, Entry{Book: book, CoverFound: coverFound(coversDir, book.Cover)})
		return nil
	})
	if walkErr != nil {
		return Catalog{}, walkErr
	}
	Sort(catalog.Entries)
	return catalog, nil
}

func isMarkdown(name string) bool {
	return strings.EqualFold(filepath.Ext(name), ".md")
}

// coverFound accepts only a readable supported image opened beneath the
// configured covers root. Missing, invalid, empty, and escaping symlink
// targets are treated as absent covers (FR-005, FR-012).
func coverFound(coversDir, cover string) bool {
	if !IsImageName(cover) {
		return false
	}
	root, err := os.OpenRoot(coversDir)
	if err != nil {
		return false
	}
	defer root.Close()
	file, err := root.Open(cover)
	if err != nil {
		return false
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 || info.Mode().Perm()&0444 == 0 {
		return false
	}
	if err := validateImage(file, cover); err != nil {
		return false
	}
	return true
}

func validateImage(file *os.File, name string) error {
	ext := strings.ToLower(filepath.Ext(name))
	if ext == ".webp" {
		var header [20]byte
		n, err := io.ReadFull(file, header[:])
		if err != nil || n < len(header) || string(header[:4]) != "RIFF" || string(header[8:12]) != "WEBP" {
			return io.ErrUnexpectedEOF
		}
		chunk := string(header[12:16])
		chunkSize := binary.LittleEndian.Uint32(header[16:20])
		riffSize := binary.LittleEndian.Uint32(header[4:8])
		info, statErr := file.Stat()
		if statErr != nil || (chunk != "VP8 " && chunk != "VP8L" && chunk != "VP8X") || chunkSize == 0 || riffSize < 12+chunkSize || int64(riffSize)+8 > info.Size() {
			return io.ErrUnexpectedEOF
		}
	} else {
		if _, _, err := image.DecodeConfig(file); err != nil {
			return err
		}
	}
	_, err := file.Seek(0, io.SeekStart)
	return err
}

// Sort orders the shelf newest-finished first, undated books after every
// dated one, then title case-insensitively, then filename for determinism
// (FR-006).
func Sort(entries []Entry) {
	sort.SliceStable(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if a.Finished.IsZero() != b.Finished.IsZero() {
			return !a.Finished.IsZero()
		}
		if !a.Finished.Equal(b.Finished) {
			return a.Finished.After(b.Finished)
		}
		if left, right := strings.ToLower(a.Title), strings.ToLower(b.Title); left != right {
			return left < right
		}
		return a.Filename < b.Filename
	})
}
