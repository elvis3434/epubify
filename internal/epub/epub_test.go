package epub

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/elvis3434/epubify/internal/book"
)

func testBook() *book.Book {
	cover := &book.Asset{Href: "images/img-0001.jpg", MediaType: "image/jpeg", Data: []byte("\xFF\xD8\xFFjpeg")}
	return &book.Book{
		Meta: book.Metadata{
			Title:      "書 & <名>",
			Authors:    []string{"甲", "乙"},
			Language:   "zh-Hant",
			Identifier: "urn:uuid:00000000-0000-5000-8000-000000000000",
			Modified:   time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		},
		Chapters: []book.Chapter{
			{File: book.ChapterFile(1), Title: "卷一", Level: 1, Class: "txt", Body: "<h1>卷一</h1>"},
			{File: book.ChapterFile(2), Title: "第一章", Level: 2, Class: "txt", Body: "<p>文字</p>",
				Sections: []book.Section{{Title: "一節", ID: "sec-1"}}},
			{File: book.ChapterFile(3), Title: "尾聲", Level: 1, Body: "<p>x</p>"},
		},
		Assets: []*book.Asset{cover},
		Cover:  cover,
	}
}

func write(t *testing.T, b *book.Book) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := Write(&buf, b); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func files(t *testing.T, data []byte) (*zip.Reader, map[string]string) {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		m[f.Name] = string(b)
	}
	return zr, m
}

func TestMimetypeEntry(t *testing.T) {
	data := write(t, testBook())
	// The local file header of the first entry: no flags, stored, no extra
	// field, then the name and the content at fixed offsets.
	if sig := binary.LittleEndian.Uint32(data[0:]); sig != 0x04034b50 {
		t.Fatalf("signature = %x", sig)
	}
	if flags := binary.LittleEndian.Uint16(data[6:]); flags != 0 {
		t.Errorf("flags = %x; want 0", flags)
	}
	if method := binary.LittleEndian.Uint16(data[8:]); method != zip.Store {
		t.Errorf("method = %d; want stored", method)
	}
	if extra := binary.LittleEndian.Uint16(data[28:]); extra != 0 {
		t.Errorf("extra field length = %d; want 0", extra)
	}
	if got := string(data[30:38]); got != "mimetype" {
		t.Errorf("first entry = %q", got)
	}
	if got := string(data[38:58]); got != "application/epub+zip" {
		t.Errorf("mimetype content = %q", got)
	}
}

func TestPackageContents(t *testing.T) {
	zr, m := files(t, write(t, testBook()))
	for _, name := range []string{
		"mimetype", "META-INF/container.xml", "OEBPS/content.opf", "OEBPS/nav.xhtml", "OEBPS/toc.ncx",
		"OEBPS/style.css", "OEBPS/cover.xhtml", "OEBPS/chapter-0001.xhtml", "OEBPS/chapter-0002.xhtml",
		"OEBPS/chapter-0003.xhtml", "OEBPS/images/img-0001.jpg",
	} {
		if _, ok := m[name]; !ok {
			t.Errorf("missing %s", name)
		}
	}
	for _, f := range zr.File {
		if len(f.Extra) != 0 {
			t.Errorf("%s has an extra field", f.Name)
		}
		if strings.HasSuffix(f.Name, ".jpg") && f.Method != zip.Store {
			t.Errorf("%s is compressed", f.Name)
		}
		if strings.HasSuffix(f.Name, ".xhtml") || strings.HasSuffix(f.Name, ".opf") || strings.HasSuffix(f.Name, ".ncx") || strings.HasSuffix(f.Name, ".xml") {
			checkWellFormed(t, f.Name, m[f.Name])
		}
	}

	opf := m["OEBPS/content.opf"]
	for _, want := range []string{
		`<dc:title>書 &amp; &lt;名&gt;</dc:title>`,
		`<dc:creator id="creator-2">乙</dc:creator>`,
		`<meta property="dcterms:modified">2026-01-02T03:04:05Z</meta>`,
		`<meta name="cover" content="img-0001"/>`,
		`<item id="img-0001" href="images/img-0001.jpg" media-type="image/jpeg" properties="cover-image"/>`,
		`<itemref idref="cover"/>`,
		`<spine toc="ncx">`,
	} {
		if !strings.Contains(opf, want) {
			t.Errorf("content.opf lacks %s\n%s", want, opf)
		}
	}
	if strings.Index(opf, `idref="cover"`) > strings.Index(opf, `idref="nav"`) {
		t.Error("the cover is not the first spine item")
	}

	nav := m["OEBPS/nav.xhtml"]
	wantNav := `<li><a href="chapter-0001.xhtml">卷一</a>
<ol>
<li><a href="chapter-0002.xhtml">第一章</a>
<ol>
<li><a href="chapter-0002.xhtml#sec-1">一節</a></li>
</ol>
</li>
</ol>
</li>
<li><a href="chapter-0003.xhtml">尾聲</a></li>
</ol>`
	if !strings.Contains(nav, wantNav) {
		t.Errorf("nav.xhtml lacks the nested TOC\n%s", nav)
	}
	if !strings.Contains(nav, "<h1>目錄</h1>") {
		t.Errorf("nav.xhtml has no localized heading")
	}

	ncx := m["OEBPS/toc.ncx"]
	if !strings.Contains(ncx, `<meta name="dtb:depth" content="3"/>`) || !strings.Contains(ncx, `playOrder="4"`) {
		t.Errorf("toc.ncx:\n%s", ncx)
	}
	if !strings.Contains(m["OEBPS/chapter-0001.xhtml"], `<body class="txt">`) {
		t.Errorf("chapter body class missing")
	}
}

func TestBuildTOC(t *testing.T) {
	var chapters []book.Chapter
	for i, level := range []int{2, 1, 4, 1} {
		chapters = append(chapters, book.Chapter{File: book.ChapterFile(i + 1), Title: string(rune('a' + i)), Level: level})
	}
	roots, depth := buildTOC(chapters)
	var sb strings.Builder
	var dump func([]*navPoint)
	dump = func(ps []*navPoint) {
		for _, p := range ps {
			fmt.Fprintf(&sb, "%s%d", p.Title, p.Order)
			if len(p.Children) > 0 {
				sb.WriteString("(")
				dump(p.Children)
				sb.WriteString(")")
			}
		}
	}
	dump(roots)
	// A level deeper than one below its predecessor is clamped.
	if got, want := sb.String(), "a1b2(c3)d4"; got != want || depth != 2 {
		t.Errorf("TOC = %s, depth %d; want %s, depth 2", got, depth, want)
	}
}

func TestVerticalAndCSS(t *testing.T) {
	b := testBook()
	b.Vertical = true
	b.CSS = "p { color: red; }"
	_, m := files(t, write(t, b))
	if opf := m["OEBPS/content.opf"]; !strings.Contains(opf, `page-progression-direction="rtl"`) ||
		!strings.Contains(opf, `<meta name="primary-writing-mode" content="vertical-rl"/>`) {
		t.Errorf("content.opf is not vertical:\n%s", opf)
	}
	css := m["OEBPS/style.css"]
	if !strings.Contains(css, "writing-mode: vertical-rl") || !strings.HasSuffix(css, "p { color: red; }\n") {
		t.Errorf("style.css:\n%s", css)
	}
}

func TestDeterministic(t *testing.T) {
	if !bytes.Equal(write(t, testBook()), write(t, testBook())) {
		t.Error("two writes of the same book differ")
	}
}

func TestWriteRejectsIncompleteBooks(t *testing.T) {
	b := testBook()
	b.Chapters = nil
	if err := Write(io.Discard, b); err == nil {
		t.Error("Write() without chapters succeeded")
	}
	b = testBook()
	b.Meta.Title = ""
	if err := Write(io.Discard, b); err == nil {
		t.Error("Write() without a title succeeded")
	}
}

func checkWellFormed(t *testing.T, name, content string) {
	t.Helper()
	d := xml.NewDecoder(strings.NewReader(content))
	for {
		_, err := d.Token()
		if err == io.EOF {
			return
		}
		if err != nil {
			t.Errorf("%s is not well-formed: %v", name, err)
			return
		}
	}
}
