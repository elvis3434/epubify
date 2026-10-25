// Package epubify converts plain text and Markdown files into EPUB 3 books.
//
//	err := epubify.ConvertFile("novel.epub", []string{"novel.txt"}, epubify.Options{
//		Authors: []string{"作者"},
//	})
package epubify

import (
	"bufio"
	"crypto/sha1"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"golang.org/x/text/language"

	"github.com/elvis3434/epubify/internal/book"
	"github.com/elvis3434/epubify/internal/epub"
	"github.com/elvis3434/epubify/internal/markdown"
	"github.com/elvis3434/epubify/internal/textenc"
	"github.com/elvis3434/epubify/internal/txt"
)

// DefaultLanguage is the book language when none is given.
const DefaultLanguage = "zh-Hant"

// Options control the conversion. The zero value is usable. Metadata given
// here overrides the front matter of Markdown files and what is detected
// in text files.
type Options struct {
	Title       string
	Authors     []string
	Language    string // BCP 47 tag; default DefaultLanguage
	Identifier  string // default: a UUID derived from the title and authors
	Publisher   string
	Description string
	Cover       string // path to a cover image

	// CSS is appended to the default stylesheet.
	CSS string
	// Vertical sets vertical writing (vertical-rl) with right-to-left
	// page progression.
	Vertical bool

	// Encoding of the input files, e.g. "big5" or "gbk". Empty or "auto"
	// detects it.
	Encoding string
	// ChapterPattern is a regular expression that matches the chapter
	// headings of text files. Empty uses the built-in patterns.
	ChapterPattern string
	// Paragraph is how text files are split into paragraphs: "auto"
	// (default), "line" or "blank".
	Paragraph string

	// Modified is the last-modified date written into the book. The zero
	// value means now.
	Modified time.Time
	// Logf receives warnings about content that was dropped or guessed.
	// It may be nil.
	Logf func(format string, args ...any)
}

// Summary describes a converted book.
type Summary struct {
	Title    string
	Chapters int
	Images   int // embedded image files, the cover included
}

type fileKind int

const (
	kindText fileKind = iota + 1
	kindMarkdown
)

func kindOf(path string) fileKind {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".txt", ".text":
		return kindText
	case ".md", ".markdown", ".mdown", ".mkd":
		return kindMarkdown
	}
	return 0
}

// ConvertFile converts the inputs and writes the book to output. The file
// is replaced only when the conversion succeeds.
func ConvertFile(output string, inputs []string, opts Options) (Summary, error) {
	dir := filepath.Dir(output)
	tmp, err := os.CreateTemp(dir, ".epubify-*.tmp")
	if err != nil {
		return Summary{}, err
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename

	w := bufio.NewWriter(tmp)
	sum, err := Convert(w, inputs, opts)
	if err == nil {
		err = w.Flush()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(tmp.Name(), 0o644)
	}
	if err == nil {
		err = os.Rename(tmp.Name(), output)
	}
	return sum, err
}

// Convert converts the inputs into one book written to w. Inputs are .txt
// and .md files, directories (their .txt and .md files are used in natural
// order: 2.md before 10.md) or glob patterns. The files become chapters in
// the order given.
func Convert(w io.Writer, inputs []string, opts Options) (Summary, error) {
	logf := opts.Logf
	if logf == nil {
		logf = func(string, ...any) {}
	}
	files, err := expandInputs(inputs)
	if err != nil {
		return Summary{}, err
	}
	paraMode, err := txt.ParseParagraphMode(opts.Paragraph)
	if err != nil {
		return Summary{}, err
	}
	lang := opts.Language

	var (
		chapters []book.Chapter
		assets   book.AssetSet
		found    struct {
			title, lang, cover, coverDir string
			description, publisher, date string
			identifier                   string
			authors                      []string
		}
	)
	setOnce := func(dst *string, v string) {
		if *dst == "" {
			*dst = v
		}
	}

	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return Summary{}, err
		}
		text, enc, err := textenc.Decode(data, opts.Encoding)
		if err != nil {
			return Summary{}, fmt.Errorf("%s: %w", f, err)
		}
		if enc != "utf-8" {
			logf("%s: read as %s", f, enc)
		}
		stem := strings.TrimSuffix(filepath.Base(f), filepath.Ext(f))

		switch kindOf(f) {
		case kindText:
			res, err := txt.Parse(text, txt.Options{
				ChapterPattern: opts.ChapterPattern,
				Paragraph:      paraMode,
				Language:       firstNonEmpty(lang, DefaultLanguage),
				FallbackTitle:  stem,
			})
			if err != nil {
				return Summary{}, fmt.Errorf("%s: %w", f, err)
			}
			for _, ch := range res.Chapters {
				ch.File = book.ChapterFile(len(chapters) + 1)
				chapters = append(chapters, ch)
			}
			setOnce(&found.title, res.Title)
			if found.authors == nil && res.Author != "" {
				found.authors = []string{res.Author}
			}

		case kindMarkdown:
			doc, err := markdown.Parse([]byte(text), markdown.Options{
				Dir:           filepath.Dir(f),
				Assets:        &assets,
				FirstChapter:  len(chapters) + 1,
				FallbackTitle: stem,
				Logf: func(format string, args ...any) {
					logf("%s: "+format, append([]any{f}, args...)...)
				},
			})
			if err != nil {
				return Summary{}, fmt.Errorf("%s: %w", f, err)
			}
			chapters = append(chapters, doc.Chapters...)
			m := doc.Meta
			setOnce(&found.title, m.Title)
			setOnce(&found.lang, m.Language)
			if found.cover == "" && m.Cover != "" {
				found.cover, found.coverDir = m.Cover, filepath.Dir(f)
			}
			setOnce(&found.description, m.Description)
			setOnce(&found.publisher, m.Publisher)
			setOnce(&found.date, m.Date)
			setOnce(&found.identifier, m.Identifier)
			if found.authors == nil && len(m.Authors) > 0 {
				found.authors = m.Authors
			}
		}
	}
	if len(chapters) == 0 {
		return Summary{}, errors.New("the input contains no text")
	}

	lang = firstNonEmpty(opts.Language, found.lang, DefaultLanguage)
	tag, err := language.Parse(lang)
	if err != nil {
		return Summary{}, fmt.Errorf("invalid language %q", lang)
	}

	b := &book.Book{
		Meta: book.Metadata{
			Title:       firstNonEmpty(opts.Title, found.title, defaultTitle(inputs, files)),
			Authors:     opts.Authors,
			Language:    tag.String(),
			Identifier:  firstNonEmpty(opts.Identifier, found.identifier),
			Publisher:   firstNonEmpty(opts.Publisher, found.publisher),
			Description: firstNonEmpty(opts.Description, found.description),
			Date:        found.date,
			Modified:    opts.Modified,
		},
		Chapters: chapters,
		Vertical: opts.Vertical,
		CSS:      opts.CSS,
	}
	if len(b.Meta.Authors) == 0 {
		b.Meta.Authors = found.authors
	}
	if b.Meta.Identifier == "" {
		b.Meta.Identifier = "urn:uuid:" + nameUUID(b.Meta.Title, b.Meta.Authors)
	}
	if b.Meta.Modified.IsZero() {
		b.Meta.Modified = time.Now()
	}

	cover := opts.Cover
	if cover == "" && found.cover != "" {
		cover = found.cover
		if !filepath.IsAbs(cover) {
			cover = filepath.Join(found.coverDir, filepath.FromSlash(cover))
		}
	}
	if cover != "" {
		if b.Cover, err = assets.AddImage(cover); err != nil {
			return Summary{}, fmt.Errorf("cover: %w", err)
		}
	}
	b.Assets = assets.List()

	if err := epub.Write(w, b); err != nil {
		return Summary{}, err
	}
	return Summary{Title: b.Meta.Title, Chapters: len(b.Chapters), Images: len(b.Assets)}, nil
}

// expandInputs resolves directories and glob patterns into a list of
// .txt and .md files.
func expandInputs(inputs []string) ([]string, error) {
	if len(inputs) == 0 {
		return nil, errors.New("no input files")
	}
	var files []string
	for _, in := range inputs {
		info, err := os.Stat(in)
		switch {
		case err == nil && info.IsDir():
			found, err := filesInDir(in)
			if err != nil {
				return nil, err
			}
			if len(found) == 0 {
				return nil, fmt.Errorf("%s: no .txt or .md files in the directory", in)
			}
			files = append(files, found...)
		case err == nil:
			if kindOf(in) == 0 {
				return nil, fmt.Errorf("%s: unsupported file type (want .txt or .md)", in)
			}
			files = append(files, in)
		case strings.ContainsAny(in, "*?["):
			// Shells on Windows do not expand patterns.
			matches, gerr := filepath.Glob(in)
			if gerr != nil {
				return nil, fmt.Errorf("%s: %w", in, gerr)
			}
			var found []string
			for _, m := range matches {
				// Like shells, skip hidden files.
				if kindOf(m) != 0 && !strings.HasPrefix(filepath.Base(m), ".") {
					found = append(found, m)
				}
			}
			if len(found) == 0 {
				return nil, fmt.Errorf("%s: no matching .txt or .md files", in)
			}
			slices.SortFunc(found, naturalCompare)
			files = append(files, found...)
		default:
			return nil, err
		}
	}
	return files, nil
}

func filesInDir(dir string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path != dir && strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.IsDir() && kindOf(path) != 0 {
			files = append(files, path)
		}
		return nil
	})
	slices.SortFunc(files, naturalCompare)
	return files, err
}

// naturalCompare orders strings so that runs of digits compare by their
// numeric value: "2.md" < "10.md". Strings that differ only in leading
// zeros are ordered by plain comparison, so the order is total.
func naturalCompare(a, b string) int {
	if c := naturalCompareValues(a, b); c != 0 {
		return c
	}
	return strings.Compare(a, b)
}

func naturalCompareValues(a, b string) int {
	for a != "" && b != "" {
		da, db := digitPrefix(a), digitPrefix(b)
		if da != "" && db != "" {
			na, nb := strings.TrimLeft(da, "0"), strings.TrimLeft(db, "0")
			if c := len(na) - len(nb); c != 0 {
				return c
			}
			if c := strings.Compare(na, nb); c != 0 {
				return c
			}
			a, b = a[len(da):], b[len(db):]
			continue
		}
		if a[0] != b[0] {
			return int(a[0]) - int(b[0])
		}
		a, b = a[1:], b[1:]
	}
	return len(a) - len(b)
}

func digitPrefix(s string) string {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	return s[:i]
}

// defaultTitle names the book after the first input: a directory's name
// or a file's name without its extension.
func defaultTitle(inputs, files []string) string {
	if info, err := os.Stat(inputs[0]); err == nil && info.IsDir() {
		if abs, err := filepath.Abs(inputs[0]); err == nil {
			return filepath.Base(abs)
		}
	}
	return strings.TrimSuffix(filepath.Base(files[0]), filepath.Ext(files[0]))
}

// nameUUID returns a version 5 (name-based) UUID, so that converting the
// same book again gives it the same identifier and reading systems treat
// it as an update.
func nameUUID(title string, authors []string) string {
	// Namespace for epubify book identifiers.
	ns := []byte{0x6f, 0x1c, 0x3a, 0x52, 0x9b, 0x0e, 0x4d, 0x8a, 0xa4, 0x63, 0x2f, 0x5e, 0x77, 0x10, 0xc9, 0xd1}
	h := sha1.New()
	h.Write(ns)
	h.Write([]byte(title))
	for _, a := range authors {
		h.Write([]byte{0})
		h.Write([]byte(a))
	}
	u := h.Sum(nil)[:16]
	u[6] = u[6]&0x0f | 0x50
	u[8] = u[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", u[0:4], u[4:6], u[6:8], u[8:10], u[10:16])
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}
