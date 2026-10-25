// Package markdown converts Markdown into XHTML chapters.
//
// The document is split at its top heading level: every heading of that
// level starts a new chapter, and the headings one level below it become
// sections in the table of contents.
package markdown

import (
	"bytes"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"

	"github.com/elvis3434/epubify/internal/book"
)

// Options control the conversion.
type Options struct {
	// Dir is the directory that relative image paths are resolved against.
	Dir string
	// Assets receives the images the document refers to.
	Assets *book.AssetSet
	// FirstChapter is the number of the first chapter file (1-based), so
	// that links between chapters can be written.
	FirstChapter int
	// FallbackTitle is the title for content before the first heading.
	FallbackTitle string
	// Logf reports content that had to be dropped. It may be nil.
	Logf func(format string, args ...any)
}

// Document is a converted Markdown file.
type Document struct {
	Meta     FrontMatter
	Chapters []book.Chapter
}

// Parse converts Markdown source into chapters.
func Parse(src []byte, opts Options) (*Document, error) {
	fm, body, err := splitFrontMatter(src)
	if err != nil {
		return nil, err
	}
	if opts.FirstChapter < 1 {
		opts.FirstChapter = 1
	}
	if opts.Logf == nil {
		opts.Logf = func(string, ...any) {}
	}

	notes := &footnoteRenderer{}
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.NewTable(extension.WithTableCellAlignMethod(extension.TableCellAlignStyle)),
			extension.Strikethrough,
			extension.Linkify,
			extension.TaskList,
			extension.Footnote,
			extension.CJK,
		),
		goldmark.WithRendererOptions(
			html.WithXHTML(),
			// Replaces the footnote renderer of extension.Footnote (priority 500).
			renderer.WithNodeRenderers(util.Prioritized(notes, 100)),
		),
	)
	doc := md.Parser().Parse(text.NewReader(body)).(*ast.Document)

	// Footnotes are distributed to the chapters after the split.
	var footnotes *east.FootnoteList
	if last, ok := doc.LastChild().(*east.FootnoteList); ok {
		doc.RemoveChild(doc, last)
		footnotes = last
	}

	c := &converter{opts: opts, src: body}
	c.fixImagesAndLinks(doc)

	title := firstNonEmpty(fm.Title, opts.FallbackTitle)
	parts := c.split(doc, title)
	notes.place(parts, footnotes)

	res := &Document{Meta: fm}
	for _, p := range parts {
		var buf bytes.Buffer
		if err := md.Renderer().Render(&buf, body, p.doc); err != nil {
			return nil, err
		}
		res.Chapters = append(res.Chapters, book.Chapter{
			File:     p.file,
			Title:    p.title,
			Level:    1,
			Class:    "md",
			Body:     buf.String(),
			Sections: p.sections,
		})
	}
	return res, nil
}

type converter struct {
	opts Options
	src  []byte
}

// part is the AST of one chapter.
type part struct {
	file     string
	title    string
	doc      *ast.Document
	sections []book.Section
}

// split moves the top-level nodes of doc into one document per chapter.
func (c *converter) split(doc *ast.Document, fallbackTitle string) []*part {
	level := 0
	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		if h, ok := n.(*ast.Heading); ok && (level == 0 || h.Level < level) {
			level = h.Level
		}
	}

	var nodes []ast.Node
	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		nodes = append(nodes, n)
	}

	var parts []*part
	var cur *part
	start := func(title string) {
		cur = &part{
			file:  book.ChapterFile(c.opts.FirstChapter + len(parts)),
			title: title,
			doc:   ast.NewDocument(),
		}
		parts = append(parts, cur)
	}
	for _, n := range nodes {
		if h, ok := n.(*ast.Heading); ok {
			switch h.Level {
			case level:
				start(firstNonEmpty(plainText(h, c.src), fallbackTitle))
			case level + 1:
				if cur == nil {
					start(fallbackTitle)
				}
				id := fmt.Sprintf("sec-%d", len(cur.sections)+1)
				h.SetAttributeString("id", []byte(id))
				if t := plainText(h, c.src); t != "" {
					cur.sections = append(cur.sections, book.Section{Title: t, ID: id})
				}
			}
		}
		if cur == nil {
			start(fallbackTitle)
		}
		cur.doc.AppendChild(cur.doc, n)
	}
	if len(parts) == 0 {
		// An empty document still makes a (blank) chapter.
		start(fallbackTitle)
	}
	return parts
}

// fixImagesAndLinks embeds local images and removes references that an
// EPUB cannot contain: remote images and links to local files.
func (c *converter) fixImagesAndLinks(doc *ast.Document) {
	var images []*ast.Image
	var links []*ast.Link
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			switch n := n.(type) {
			case *ast.Image:
				images = append(images, n)
			case *ast.Link:
				links = append(links, n)
			}
		}
		return ast.WalkContinue, nil
	})

	for _, img := range images {
		dest := string(img.Destination)
		if href, ok := c.embedImage(dest); ok {
			img.Destination = []byte(href)
			continue
		}
		// Replace the image with its alternative text.
		parent := img.Parent()
		if alt := plainText(img, c.src); alt != "" {
			parent.ReplaceChild(parent, img, ast.NewString([]byte(alt)))
		} else {
			parent.RemoveChild(parent, img)
			if _, ok := parent.(*ast.Paragraph); ok && parent.ChildCount() == 0 {
				parent.Parent().RemoveChild(parent.Parent(), parent)
			}
		}
	}

	for _, link := range links {
		dest := string(link.Destination)
		if strings.HasPrefix(dest, "#") || hasScheme(dest) {
			continue
		}
		c.opts.Logf("link to %q removed: links to local files are not supported", dest)
		parent := link.Parent()
		for child := link.FirstChild(); child != nil; {
			next := child.NextSibling()
			parent.InsertBefore(parent, link, child)
			child = next
		}
		parent.RemoveChild(parent, link)
	}
}

// embedImage adds the image at dest to the assets and returns its href in
// the package.
func (c *converter) embedImage(dest string) (string, bool) {
	switch {
	case dest == "":
		return "", false
	case hasScheme(dest) || strings.HasPrefix(dest, "//"):
		c.opts.Logf("image %q replaced by its text: remote images are not allowed in EPUB", dest)
		return "", false
	}
	p := dest
	if i := strings.IndexAny(p, "?#"); i >= 0 {
		p = p[:i]
	}
	if u, err := url.PathUnescape(p); err == nil {
		p = u
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(c.opts.Dir, filepath.FromSlash(p))
	}
	a, err := c.opts.Assets.AddImage(p)
	if err != nil {
		c.opts.Logf("image %q replaced by its text: %v", dest, err)
		return "", false
	}
	return a.Href, true
}

// hasScheme reports whether dest is an absolute URL such as https://... or
// mailto:... A Windows drive letter is not a scheme.
func hasScheme(dest string) bool {
	u, err := url.Parse(dest)
	return err == nil && len(u.Scheme) > 1
}

// plainText returns the text content of an inline container.
func plainText(n ast.Node, src []byte) string {
	var sb strings.Builder
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch c := c.(type) {
		case *ast.Text:
			sb.Write(c.Value(src))
			if c.SoftLineBreak() || c.HardLineBreak() {
				sb.WriteByte(' ')
			}
		case *ast.String:
			sb.Write(c.Value)
		case *east.FootnoteLink:
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	s := util.UnescapePunctuations([]byte(sb.String()))
	s = util.ResolveNumericReferences(s)
	s = util.ResolveEntityNames(s)
	return strings.TrimSpace(string(s))
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}
