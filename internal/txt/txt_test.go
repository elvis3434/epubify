package txt

import (
	"strings"
	"testing"

	"github.com/elvis3434/epubify/internal/book"
)

func parse(t *testing.T, text string, opts Options) *Result {
	t.Helper()
	res, err := Parse(text, opts)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func titles(chapters []book.Chapter) []string {
	var out []string
	for _, ch := range chapters {
		out = append(out, strings.Repeat(">", ch.Level-1)+ch.Title)
	}
	return out
}

func equal(t *testing.T, got, want []string) {
	t.Helper()
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestHeadings(t *testing.T) {
	isHeading, err := headingMatcher("")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		line         string
		heading, vol bool
	}{
		{"第一章 車站", true, false},
		{"第12章", true, false},
		{"第１２３回 決戰", true, false},
		{"第一百零八章　大結局", true, false},
		{"第三卷 雨夜", true, true},
		{"卷一", true, true},
		{"第二部 歸來", true, true},
		{"Chapter 7", true, false},
		{"CHAPTER IV The Storm", true, false},
		{"Part 2", true, true},
		{"序章", true, false},
		{"序：寫在前面", true, false},
		{"楔子", true, false},
		{"後記", true, false},
		{"番外一 夏日", true, false},
		{"Prologue", true, false},
		// Prose
		{"序幕拉開，觀眾安靜下來。", false, false},
		{"第一次見面的時候，他什麼也沒說", false, false},
		{"第三回合開始了。", false, false},
		{"第二天早上", false, false},
		{"「第一章寫完了嗎？」", false, false},
		{"第一章" + strings.Repeat("很長", 30), false, false},
		{"", false, false},
	}
	for _, tt := range tests {
		ok, vol := isHeading(tt.line)
		if ok != tt.heading || vol != tt.vol {
			t.Errorf("isHeading(%q) = %v, %v; want %v, %v", tt.line, ok, vol, tt.heading, tt.vol)
		}
	}
}

func TestParseChapters(t *testing.T) {
	text := "《山城》\n作者：林小雨\n\n簡介。\n\n第一章 車站\n\n　　火車進站。\n　　他下車了。\n\n第二章 老街\n老街很窄。\n"
	res := parse(t, text, Options{FallbackTitle: "file"})
	if res.Title != "山城" || res.Author != "林小雨" {
		t.Errorf("title, author = %q, %q", res.Title, res.Author)
	}
	equal(t, titles(res.Chapters), []string{"山城", "第一章 車站", "第二章 老街"})

	want := "<h2 class=\"chapter\">第一章 車站</h2>\n<p>火車進站。</p>\n<p>他下車了。</p>\n"
	if got := res.Chapters[1].Body; got != want {
		t.Errorf("body = %q; want %q", got, want)
	}
	for _, ch := range res.Chapters {
		if ch.Class != "txt" {
			t.Errorf("class = %q", ch.Class)
		}
	}
}

func TestParseVolumes(t *testing.T) {
	text := "第一卷 歸途\n第一章 車站\n文字。\n第二章 老街\n文字。\n第二卷 雨夜\n第三章 燈火\n文字。\n"
	res := parse(t, text, Options{})
	equal(t, titles(res.Chapters), []string{"第一卷 歸途", ">第一章 車站", ">第二章 老街", "第二卷 雨夜", ">第三章 燈火"})
	if !strings.HasPrefix(res.Chapters[0].Body, `<h1 class="volume">`) {
		t.Errorf("volume body = %q", res.Chapters[0].Body)
	}

	// Volumes alone are not nested.
	res = parse(t, "第一部\n文字。\n第二部\n文字。\n", Options{})
	equal(t, titles(res.Chapters), []string{"第一部", "第二部"})
}

func TestParseDropsCopiedTableOfContents(t *testing.T) {
	text := "書名\n\n目錄\n第一卷 歸途\n第一章 車站\n第二章 老街\n\n第一卷 歸途\n\n第一章 車站\n文字。\n第二章 老街\n文字。\n"
	res := parse(t, text, Options{FallbackTitle: "book"})
	equal(t, titles(res.Chapters), []string{"book", "第一卷 歸途", ">第一章 車站", ">第二章 老街"})
	if got := res.Chapters[0].Body; got != "<p>書名</p>\n" {
		t.Errorf("preamble = %q", got)
	}

	// Two empty headings in a row are kept.
	res = parse(t, "第一章\n第二章\n文字。\n", Options{})
	equal(t, titles(res.Chapters), []string{"第一章", "第二章"})
}

func TestParseCustomPattern(t *testing.T) {
	text := "== 開始 ==\n文字。\n== 結束 ==\n文字。\n第一章 不是標題\n"
	res := parse(t, text, Options{ChapterPattern: `^== .+ ==$`})
	equal(t, titles(res.Chapters), []string{"== 開始 ==", "== 結束 =="})

	if _, err := Parse("x", Options{ChapterPattern: "("}); err == nil {
		t.Error("Parse() with an invalid pattern succeeded")
	}
}

func TestParseWithoutHeadings(t *testing.T) {
	res := parse(t, "一段。\n兩段。\n", Options{FallbackTitle: "短文"})
	equal(t, titles(res.Chapters), []string{"短文"})

	long := strings.Repeat(strings.Repeat("字", 100)+"。\n", 25) // 2500 characters
	res = parse(t, long, Options{ChunkSize: 1000, Language: "zh-Hant"})
	equal(t, titles(res.Chapters), []string{"第 1 部分", "第 2 部分", "第 3 部分"})
	res = parse(t, long, Options{ChunkSize: 1000, Language: "en"})
	equal(t, titles(res.Chapters), []string{"Part 1", "Part 2", "Part 3"})

	if res := parse(t, " \n\n", Options{}); len(res.Chapters) != 0 {
		t.Errorf("blank text gave %d chapters", len(res.Chapters))
	}
}

func TestParagraphModes(t *testing.T) {
	wrapped := "This is a paragraph that was\nwrapped at a fixed width and\ncontinues on a third line\n\nA second paragraph also\nwraps over several lines\nof text in the file\n"
	res := parse(t, wrapped, Options{})
	want := "<p>This is a paragraph that was wrapped at a fixed width and continues on a third line</p>\n" +
		"<p>A second paragraph also wraps over several lines of text in the file</p>\n"
	if got := res.Chapters[0].Body; got != want {
		t.Errorf("auto (wrapped) = %q", got)
	}

	// Hard-wrapped CJK text joins without spaces; indentation starts a paragraph.
	cjk := "　　這是一段被固定寬度\n切開的文字，沒有空行\n分隔段落\n　　第二段從縮排開始\n繼續寫下去\n"
	res = parse(t, cjk, Options{Paragraph: ParagraphBlank})
	want = "<p>這是一段被固定寬度切開的文字，沒有空行分隔段落</p>\n<p>第二段從縮排開始繼續寫下去</p>\n"
	if got := res.Chapters[0].Body; got != want {
		t.Errorf("blank (CJK) = %q", got)
	}

	lines := "第一行。\n第二行。\n第三行。\n"
	res = parse(t, lines, Options{})
	if got := strings.Count(res.Chapters[0].Body, "<p>"); got != 3 {
		t.Errorf("auto (one paragraph per line) gave %d paragraphs", got)
	}

	if _, err := ParseParagraphMode("words"); err == nil {
		t.Error("ParseParagraphMode accepted an unknown mode")
	}
}

func TestParseEscapesText(t *testing.T) {
	res := parse(t, "第一章 <A&B>\nx < y & \"z\"\n", Options{})
	body := res.Chapters[0].Body
	if !strings.Contains(body, "第一章 &lt;A&amp;B&gt;") || !strings.Contains(body, "x &lt; y &amp; &#34;z&#34;") {
		t.Errorf("body not escaped: %q", body)
	}
}

func TestParseLineEndings(t *testing.T) {
	res := parse(t, "第一章\r\n文字一。\r\n第二章\r文字二。\r", Options{})
	equal(t, titles(res.Chapters), []string{"第一章", "第二章"})
}
