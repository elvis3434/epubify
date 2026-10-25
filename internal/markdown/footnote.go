package markdown

import (
	"fmt"

	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
)

// footnoteRenderer renders footnotes for a document that is split into
// several files. Each footnote is placed at the end of the chapter that
// first refers to it, and every link names its target file, so references
// across chapters still work. Links carry epub:type="noteref" so reading
// systems can show the note in a pop-up.
type footnoteRenderer struct {
	noteFile map[int]string    // footnote index -> file holding the note
	refFile  map[[2]int]string // (index, reference index) -> file holding the reference
}

// place moves the footnotes of list into the chapters and records where
// every note and reference ends up.
func (r *footnoteRenderer) place(parts []*part, list *east.FootnoteList) {
	r.noteFile = map[int]string{}
	r.refFile = map[[2]int]string{}
	if list == nil {
		return
	}
	notes := map[*part][]*east.Footnote{}
	order := map[int]*part{}
	for _, p := range parts {
		_ = ast.Walk(p.doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
			if link, ok := n.(*east.FootnoteLink); ok && entering {
				r.refFile[[2]int{link.Index, link.RefIndex}] = p.file
				if _, seen := order[link.Index]; !seen {
					order[link.Index] = p
				}
			}
			return ast.WalkContinue, nil
		})
	}
	var all []*east.Footnote
	for n := list.FirstChild(); n != nil; n = n.NextSibling() {
		all = append(all, n.(*east.Footnote))
	}
	for _, fn := range all {
		p, ok := order[fn.Index]
		if !ok {
			p = parts[len(parts)-1]
		}
		notes[p] = append(notes[p], fn)
		r.noteFile[fn.Index] = p.file
	}
	for _, p := range parts {
		if len(notes[p]) == 0 {
			continue
		}
		l := east.NewFootnoteList()
		for _, fn := range notes[p] {
			l.AppendChild(l, fn)
		}
		l.Count = len(notes[p])
		p.doc.AppendChild(p.doc, l)
	}
}

func (r *footnoteRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(east.KindFootnoteLink, r.renderLink)
	reg.Register(east.KindFootnoteBacklink, r.renderBacklink)
	reg.Register(east.KindFootnote, r.renderFootnote)
	reg.Register(east.KindFootnoteList, r.renderList)
}

func (r *footnoteRenderer) renderLink(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		n := node.(*east.FootnoteLink)
		fmt.Fprintf(w, `<sup><a id="fnref-%d-%d" href="%s#fn-%d" class="footnote-ref" epub:type="noteref" role="doc-noteref">%d</a></sup>`,
			n.Index, n.RefIndex, r.noteFile[n.Index], n.Index, n.Index)
	}
	return ast.WalkContinue, nil
}

func (r *footnoteRenderer) renderBacklink(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		n := node.(*east.FootnoteBacklink)
		fmt.Fprintf(w, `&#160;<a href="%s#fnref-%d-%d" class="footnote-backref" role="doc-backlink">&#x21A9;&#xFE0E;</a>`,
			r.refFile[[2]int{n.Index, n.RefIndex}], n.Index, n.RefIndex)
	}
	return ast.WalkContinue, nil
}

func (r *footnoteRenderer) renderFootnote(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		// value keeps the document-wide number in a chapter's own list.
		i := node.(*east.Footnote).Index
		fmt.Fprintf(w, "<li id=\"fn-%d\" value=\"%d\" epub:type=\"footnote\">\n", i, i)
	} else {
		_, _ = w.WriteString("</li>\n")
	}
	return ast.WalkContinue, nil
}

func (r *footnoteRenderer) renderList(w util.BufWriter, _ []byte, _ ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.WriteString("<section class=\"footnotes\" epub:type=\"footnotes\">\n<ol>\n")
	} else {
		_, _ = w.WriteString("</ol>\n</section>\n")
	}
	return ast.WalkContinue, nil
}
