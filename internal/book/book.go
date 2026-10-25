// Package book defines the format-neutral model that the input parsers
// produce and the EPUB writer consumes.
package book

import (
	"fmt"
	"strings"
	"time"
)

// Metadata describes the publication.
type Metadata struct {
	Title       string
	Authors     []string
	Language    string // BCP 47 tag, e.g. "zh-Hant"
	Identifier  string // e.g. "urn:uuid:..."
	Publisher   string
	Description string
	Date        string // publication date (W3CDTF), optional
	Modified    time.Time
}

// Chapter is one XHTML content document of the book.
type Chapter struct {
	File     string    // file name inside the package, see ChapterFile
	Title    string    // title shown in the table of contents
	Level    int       // depth in the table of contents, 1 is the top level
	Class    string    // class attribute of <body>, selects the styling
	Body     string    // well-formed XHTML fragment placed inside <body>
	Sections []Section // anchors in Body listed in the TOC one level deeper
}

// Section is a heading inside a chapter that gets its own TOC entry.
type Section struct {
	Title string
	ID    string
}

// Asset is a binary resource stored in the package, such as an image.
type Asset struct {
	Href      string // path inside the package, e.g. "images/img-0001.png"
	MediaType string
	Data      []byte
}

// Book is everything needed to write an EPUB.
type Book struct {
	Meta     Metadata
	Chapters []Chapter
	Assets   []*Asset
	Cover    *Asset // one of Assets, or nil
	Vertical bool   // vertical-rl writing mode, right-to-left page order
	CSS      string // extra stylesheet appended after the default one
}

// ChapterFile returns the file name of the n-th chapter (1-based).
func ChapterFile(n int) string {
	return fmt.Sprintf("chapter-%04d.xhtml", n)
}

// Labels are the generated strings that appear in the book.
type Labels struct {
	Contents string
	Cover    string
	part     string
}

// Part returns the title for the n-th automatically split part.
func (l Labels) Part(n int) string {
	return fmt.Sprintf(l.part, n)
}

// LabelsFor returns the labels for a BCP 47 language tag.
func LabelsFor(lang string) Labels {
	l := strings.ToLower(lang)
	switch {
	case l == "zh" || strings.HasPrefix(l, "zh-"):
		if strings.Contains(l, "hans") || strings.HasSuffix(l, "-cn") || strings.HasSuffix(l, "-sg") {
			return Labels{Contents: "目录", Cover: "封面", part: "第 %d 部分"}
		}
		return Labels{Contents: "目錄", Cover: "封面", part: "第 %d 部分"}
	case l == "ja" || strings.HasPrefix(l, "ja-"):
		return Labels{Contents: "目次", Cover: "表紙", part: "第%d部"}
	default:
		return Labels{Contents: "Contents", Cover: "Cover", part: "Part %d"}
	}
}
