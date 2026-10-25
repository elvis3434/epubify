package markdown

import (
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elvis3434/epubify/internal/book"
)

type result struct {
	doc    *Document
	assets *book.AssetSet
	logs   []string
}

func convert(t *testing.T, src string, dir string) result {
	t.Helper()
	r := result{assets: &book.AssetSet{}}
	doc, err := Parse([]byte(src), Options{
		Dir:           dir,
		Assets:        r.assets,
		FirstChapter:  1,
		FallbackTitle: "fallback",
		Logf:          func(format string, args ...any) { r.logs = append(r.logs, fmt.Sprintf(format, args...)) },
	})
	if err != nil {
		t.Fatal(err)
	}
	r.doc = doc
	for _, ch := range doc.Chapters {
		checkWellFormed(t, ch.Body)
	}
	return r
}

// checkWellFormed fails the test if body is not well-formed XML.
func checkWellFormed(t *testing.T, body string) {
	t.Helper()
	doc := `<body xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">` + body + `</body>`
	d := xml.NewDecoder(strings.NewReader(doc))
	for {
		_, err := d.Token()
		if err == io.EOF {
			return
		}
		if err != nil {
			t.Fatalf("not well-formed XML: %v\n%s", err, body)
		}
	}
}

func titles(doc *Document) []string {
	var out []string
	for _, ch := range doc.Chapters {
		out = append(out, ch.Title)
		for _, s := range ch.Sections {
			out = append(out, ">"+s.Title+"#"+s.ID)
		}
	}
	return out
}

func equal(t *testing.T, got, want []string) {
	t.Helper()
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestFrontMatter(t *testing.T) {
	src := "---\ntitle: 書名\nauthor:\n  - 甲\n  - 乙\nlang: zh-TW\ncover: img/c.jpg\ndate: 2024-05-01\ndescription: 說明\npublisher: 出版社\nisbn: 9789570000000\n---\n# 一\n"
	r := convert(t, src, "")
	want := FrontMatter{
		Title: "書名", Authors: []string{"甲", "乙"}, Language: "zh-TW", Cover: "img/c.jpg",
		Date: "2024-05-01", Description: "說明", Publisher: "出版社", Identifier: "9789570000000",
	}
	if fmt.Sprint(r.doc.Meta) != fmt.Sprint(want) {
		t.Errorf("front matter = %+v\nwant %+v", r.doc.Meta, want)
	}
	equal(t, titles(r.doc), []string{"一"})

	r = convert(t, "---\nauthor: 單一作者\n---\ntext\n", "")
	equal(t, r.doc.Meta.Authors, []string{"單一作者"})
	equal(t, titles(r.doc), []string{"fallback"})

	// Without a closing line the dashes are a thematic break.
	r = convert(t, "---\ntext\n", "")
	if !strings.Contains(r.doc.Chapters[0].Body, "<hr />") {
		t.Errorf("body = %q", r.doc.Chapters[0].Body)
	}

	for _, bad := range []string{"---\ntitle: [unclosed\n---\n", "---\ndate: May 2024\n---\n"} {
		if _, err := Parse([]byte(bad), Options{Assets: &book.AssetSet{}}); err == nil {
			t.Errorf("Parse(%q) succeeded", bad)
		}
	}
}

func TestSplit(t *testing.T) {
	src := "Intro text.\n\n# One\n\ntext\n\n## A\n\n### deep\n\n## B\n\n# Two\n\ntext\n"
	r := convert(t, src, "")
	equal(t, titles(r.doc), []string{"fallback", "One", ">A#sec-1", ">B#sec-2", "Two"})
	for i, ch := range r.doc.Chapters {
		if want := book.ChapterFile(i + 1); ch.File != want {
			t.Errorf("chapter %d file = %q; want %q", i, ch.File, want)
		}
	}
	if body := r.doc.Chapters[1].Body; !strings.Contains(body, `<h2 id="sec-1">A</h2>`) {
		t.Errorf("section heading has no id: %q", body)
	}

	// The top level present is used for the split.
	r = convert(t, "## One\n\n### a\n\n## Two **bold** `code`\n", "")
	equal(t, titles(r.doc), []string{"One", ">a#sec-1", "Two bold code"})

	r = convert(t, "", "")
	equal(t, titles(r.doc), []string{"fallback"})
}

func TestImagesAndLinks(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "img"), 0o755); err != nil {
		t.Fatal(err)
	}
	png := []byte("\x89PNG\r\n\x1A\nrest")
	if err := os.WriteFile(filepath.Join(dir, "img", "a b.png"), png, 0o644); err != nil {
		t.Fatal(err)
	}
	src := "# T\n\n![local](img/a%20b.png) ![again](img/a%20b.png)\n\n" +
		"![missing](img/none.png)\n\n![remote](https://example.com/x.png)\n\n![](https://example.com/y.png)\n\n" +
		"[web](https://go.dev) [mail](mailto:a@example.com) [anchor](#sec-1) [local **file**](other.md)\n"
	r := convert(t, src, dir)
	body := r.doc.Chapters[0].Body

	if list := r.assets.List(); len(list) != 1 || list[0].Href != "images/img-0001.png" || list[0].MediaType != "image/png" {
		t.Fatalf("assets = %+v", list)
	}
	for _, want := range []string{
		`<img src="images/img-0001.png" alt="local" />`,
		`<img src="images/img-0001.png" alt="again" />`,
		`<p>missing</p>`,
		`<p>remote</p>`,
		`<a href="https://go.dev">web</a>`,
		`<a href="mailto:a@example.com">mail</a>`,
		`<a href="#sec-1">anchor</a>`,
		`local <strong>file</strong>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body lacks %s\n%s", want, body)
		}
	}
	if strings.Contains(body, "example.com/") || strings.Contains(body, "other.md") || strings.Contains(body, "<p></p>") {
		t.Errorf("body keeps a removed reference:\n%s", body)
	}
	if len(r.logs) != 4 {
		t.Errorf("logs = %q; want 4 entries", r.logs)
	}
}

func TestFootnotesAcrossChapters(t *testing.T) {
	src := "# One\n\nA[^a] and B[^b].\n\n# Two\n\nA again[^a], C[^c].\n\n[^a]: Note A.\n[^b]: Note B.\n[^c]: Note C.\n"
	r := convert(t, src, "")
	one, two := r.doc.Chapters[0].Body, r.doc.Chapters[1].Body
	for _, want := range []string{
		`<a id="fnref-1-0" href="chapter-0001.xhtml#fn-1"`,
		`<li id="fn-1" value="1" epub:type="footnote">`,
		`<li id="fn-2" value="2" epub:type="footnote">`,
		`href="chapter-0001.xhtml#fnref-1-0"`,
		`href="chapter-0002.xhtml#fnref-1-1"`,
	} {
		if !strings.Contains(one, want) {
			t.Errorf("chapter one lacks %s\n%s", want, one)
		}
	}
	for _, want := range []string{
		`<a id="fnref-1-1" href="chapter-0001.xhtml#fn-1"`,
		`<li id="fn-3" value="3" epub:type="footnote">`,
	} {
		if !strings.Contains(two, want) {
			t.Errorf("chapter two lacks %s\n%s", want, two)
		}
	}
	if strings.Contains(two, `id="fn-1"`) {
		t.Errorf("note 1 is repeated in chapter two")
	}
}

func TestRendering(t *testing.T) {
	src := "# T\n\n中文段落的換行\n不會產生空白。\n\n<div>raw</div>\n\nline<br>break\n\n| a | b |\n|:--|--:|\n| 1 | 2 |\n\n- [x] done\n\n&copy; &amp; &#x4E2D;\n"
	r := convert(t, src, "")
	body := r.doc.Chapters[0].Body
	for _, want := range []string{
		"<p>中文段落的換行不會產生空白。</p>",
		"<!-- raw HTML omitted -->",
		`<th style="text-align:left">a</th>`,
		`<input checked="" disabled="" type="checkbox" />`,
		"© &amp; 中",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body lacks %s\n%s", want, body)
		}
	}
	if strings.Contains(body, "<div>") || strings.Contains(body, "<br>") {
		t.Errorf("raw HTML was passed through:\n%s", body)
	}
}

func TestHeadingTitleText(t *testing.T) {
	r := convert(t, "# A \\* B &amp; C[^n]\n\n[^n]: note\n", "")
	equal(t, titles(r.doc), []string{"A * B & C"})
}
