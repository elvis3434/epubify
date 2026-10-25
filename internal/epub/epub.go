// Package epub writes a book.Book as an EPUB 3 package.
//
// The package also carries an EPUB 2 NCX table of contents so that older
// reading systems can navigate the book.
package epub

import (
	"archive/zip"
	"bytes"
	"embed"
	"errors"
	"hash/crc32"
	"html"
	"io"
	"path"
	"strings"
	"text/template"
	"time"

	"github.com/elvis3434/epubify/internal/book"
)

//go:embed templates
var templateFS embed.FS

var templates = template.Must(template.New("").Funcs(template.FuncMap{
	"esc": html.EscapeString,
	"inc": func(i int) int { return i + 1 },
}).ParseFS(templateFS, "templates/*.opf", "templates/*.xhtml", "templates/*.ncx"))

const (
	containerXML = `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>
`
	mimetype  = "application/epub+zip"
	coverFile = "cover.xhtml"
)

type manifestItem struct {
	ID, Href, MediaType, Properties string
}

// navPoint is one entry of the table of contents.
type navPoint struct {
	Title    string
	Href     string
	Order    int // play order in the NCX, 1-based
	Children []*navPoint
}

type packageData struct {
	Meta      book.Metadata
	Modified  string
	Labels    book.Labels
	Vertical  bool
	CoverID   string
	CoverFile string
	Manifest  []manifestItem
	Spine     []string
	TOC       []*navPoint
	Depth     int
}

type pageData struct {
	Lang, Title, Class, EpubType, Body string
}

// Write writes b to w as an EPUB file.
func Write(w io.Writer, b *book.Book) error {
	if len(b.Chapters) == 0 {
		return errors.New("epub: the book has no chapters")
	}
	if b.Meta.Title == "" || b.Meta.Identifier == "" || b.Meta.Language == "" {
		return errors.New("epub: title, identifier and language are required")
	}

	pkg := newPackageData(b)
	zw := zip.NewWriter(w)
	pw := &packageWriter{zw: zw}
	pw.date, pw.time = dosTime(b.Meta.Modified)

	pw.mimetype()
	pw.file("META-INF/container.xml", []byte(containerXML), true)
	pw.template("OEBPS/content.opf", "content.opf", pkg)
	pw.template("OEBPS/nav.xhtml", "nav.xhtml", pkg)
	pw.template("OEBPS/toc.ncx", "toc.ncx", pkg)
	pw.file("OEBPS/style.css", []byte(stylesheet(b)), true)
	if b.Cover != nil {
		body := `<div class="cover"><img src="` + html.EscapeString(b.Cover.Href) +
			`" alt="` + html.EscapeString(b.Meta.Title) + `"/></div>`
		pw.template("OEBPS/"+coverFile, "page.xhtml", pageData{
			Lang: b.Meta.Language, Title: pkg.Labels.Cover, Class: "cover", EpubType: "cover", Body: body,
		})
	}
	for _, ch := range b.Chapters {
		pw.template("OEBPS/"+ch.File, "page.xhtml", pageData{
			Lang: b.Meta.Language, Title: ch.Title, Class: ch.Class, Body: ch.Body,
		})
	}
	for _, a := range b.Assets {
		// Raster images are already compressed.
		pw.file("OEBPS/"+a.Href, a.Data, a.MediaType == "image/svg+xml")
	}
	if pw.err != nil {
		return pw.err
	}
	return zw.Close()
}

func newPackageData(b *book.Book) *packageData {
	pkg := &packageData{
		Meta:     b.Meta,
		Modified: b.Meta.Modified.UTC().Format("2006-01-02T15:04:05Z"),
		Labels:   book.LabelsFor(b.Meta.Language),
		Vertical: b.Vertical,
	}
	pkg.Manifest = []manifestItem{
		{ID: "nav", Href: "nav.xhtml", MediaType: "application/xhtml+xml", Properties: "nav"},
		{ID: "ncx", Href: "toc.ncx", MediaType: "application/x-dtbncx+xml"},
		{ID: "css", Href: "style.css", MediaType: "text/css"},
	}
	if b.Cover != nil {
		pkg.CoverID = assetID(b.Cover)
		pkg.CoverFile = coverFile
		pkg.Manifest = append(pkg.Manifest, manifestItem{ID: "cover", Href: coverFile, MediaType: "application/xhtml+xml"})
		pkg.Spine = append(pkg.Spine, "cover")
	}
	pkg.Spine = append(pkg.Spine, "nav")
	for _, ch := range b.Chapters {
		id := strings.TrimSuffix(ch.File, path.Ext(ch.File))
		pkg.Manifest = append(pkg.Manifest, manifestItem{ID: id, Href: ch.File, MediaType: "application/xhtml+xml"})
		pkg.Spine = append(pkg.Spine, id)
	}
	for _, a := range b.Assets {
		item := manifestItem{ID: assetID(a), Href: a.Href, MediaType: a.MediaType}
		if a == b.Cover {
			item.Properties = "cover-image"
		}
		pkg.Manifest = append(pkg.Manifest, item)
	}
	pkg.TOC, pkg.Depth = buildTOC(b.Chapters)
	return pkg
}

func assetID(a *book.Asset) string {
	base := path.Base(a.Href)
	return strings.TrimSuffix(base, path.Ext(base))
}

// buildTOC turns the chapter levels into a tree and numbers the entries in
// reading order. A level deeper than one below its predecessor is clamped,
// so the tree never skips a level.
func buildTOC(chapters []book.Chapter) (roots []*navPoint, depth int) {
	var stack []*navPoint // stack[i] is the latest entry at level i+1
	order := 0
	add := func(level int, title, href string) {
		level = max(1, min(level, len(stack)+1))
		order++
		p := &navPoint{Title: title, Href: href, Order: order}
		stack = stack[:level-1]
		if level == 1 {
			roots = append(roots, p)
		} else {
			parent := stack[level-2]
			parent.Children = append(parent.Children, p)
		}
		stack = append(stack, p)
		depth = max(depth, level)
	}
	for _, ch := range chapters {
		add(ch.Level, ch.Title, ch.File)
		for _, s := range ch.Sections {
			add(ch.Level+1, s.Title, ch.File+"#"+s.ID)
		}
	}
	return roots, depth
}

func stylesheet(b *book.Book) string {
	css := mustRead("templates/style.css")
	if b.Vertical {
		css += mustRead("templates/vertical.css")
	}
	if b.CSS != "" {
		css += "\n/* Custom */\n" + b.CSS
		if !strings.HasSuffix(css, "\n") {
			css += "\n"
		}
	}
	return css
}

func mustRead(name string) string {
	data, err := templateFS.ReadFile(name)
	if err != nil {
		panic(err)
	}
	return string(data)
}

// packageWriter adds files to the ZIP container and remembers the first
// error, so the caller can check it once at the end.
type packageWriter struct {
	zw         *zip.Writer
	date, time uint16
	err        error
}

// mimetype writes the mimetype file. It must be the first entry, stored
// uncompressed, without a data descriptor and without extra fields, so
// that reading systems can find it at a fixed offset.
func (pw *packageWriter) mimetype() {
	data := []byte(mimetype)
	fh := &zip.FileHeader{
		Name:               "mimetype",
		Method:             zip.Store,
		CRC32:              crc32.ChecksumIEEE(data),
		CompressedSize64:   uint64(len(data)),
		UncompressedSize64: uint64(len(data)),
		ModifiedDate:       pw.date,
		ModifiedTime:       pw.time,
	}
	f, err := pw.zw.CreateRaw(fh)
	if err == nil {
		_, err = f.Write(data)
	}
	pw.setErr(err)
}

func (pw *packageWriter) file(name string, data []byte, compress bool) {
	if pw.err != nil {
		return
	}
	// The MS-DOS time fields are set directly: setting FileHeader.Modified
	// would add an extended timestamp extra field to every entry.
	fh := &zip.FileHeader{Name: name, Method: zip.Store, ModifiedDate: pw.date, ModifiedTime: pw.time}
	if compress {
		fh.Method = zip.Deflate
	}
	f, err := pw.zw.CreateHeader(fh)
	if err == nil {
		_, err = f.Write(data)
	}
	pw.setErr(err)
}

func (pw *packageWriter) template(name, tmpl string, data any) {
	var buf bytes.Buffer
	if err := templates.ExecuteTemplate(&buf, tmpl, data); err != nil {
		pw.setErr(err)
		return
	}
	pw.file(name, buf.Bytes(), true)
}

func (pw *packageWriter) setErr(err error) {
	if pw.err == nil {
		pw.err = err
	}
}

// dosTime converts t to the MS-DOS date and time used by ZIP headers.
func dosTime(t time.Time) (date, tm uint16) {
	t = t.UTC()
	if t.Year() < 1980 {
		t = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	date = uint16(t.Day() + int(t.Month())<<5 + (t.Year()-1980)<<9)
	tm = uint16(t.Second()/2 + t.Minute()<<5 + t.Hour()<<11)
	return date, tm
}
