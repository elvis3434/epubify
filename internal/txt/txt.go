// Package txt splits plain text into chapters and paragraphs.
package txt

import (
	"fmt"
	"html"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/elvis3434/epubify/internal/book"
)

// ParagraphMode selects how lines are grouped into paragraphs.
type ParagraphMode string

const (
	// ParagraphAuto picks ParagraphLine or ParagraphBlank by looking at the text.
	ParagraphAuto ParagraphMode = "auto"
	// ParagraphLine makes every non-empty line a paragraph. Most Chinese
	// novels are formatted this way.
	ParagraphLine ParagraphMode = "line"
	// ParagraphBlank joins consecutive lines into one paragraph; paragraphs
	// are separated by blank lines or by an indented first line. This suits
	// hard-wrapped text.
	ParagraphBlank ParagraphMode = "blank"
)

// ParseParagraphMode validates a paragraph mode name. The empty string
// means ParagraphAuto.
func ParseParagraphMode(s string) (ParagraphMode, error) {
	switch m := ParagraphMode(strings.ToLower(s)); m {
	case "":
		return ParagraphAuto, nil
	case ParagraphAuto, ParagraphLine, ParagraphBlank:
		return m, nil
	}
	return "", fmt.Errorf("unknown paragraph mode %q (want auto, line or blank)", s)
}

// Options control the parsing.
type Options struct {
	// ChapterPattern is a regular expression matched against each trimmed
	// line to find chapter headings. Empty means the built-in patterns.
	ChapterPattern string
	Paragraph      ParagraphMode
	// Language selects the titles of automatically split parts.
	Language string
	// FallbackTitle is the title for text before the first heading and
	// for a text that has no headings at all.
	FallbackTitle string
	// ChunkSize is the approximate number of characters per part when the
	// text has no headings. Zero means DefaultChunkSize.
	ChunkSize int
}

// DefaultChunkSize is the part size used for text without headings.
const DefaultChunkSize = 10000

// Result is a parsed text. Chapter File names are left for the caller to set.
type Result struct {
	Chapters []book.Chapter
	Title    string // title found in the header lines, if any
	Author   string // author found in the header lines, if any
}

const (
	numerals = `[0-9０-９零〇一二兩两三四五六七八九十百千萬万]+`
	// separator is what may follow a special heading word such as 序章.
	separator = `(?:$|[\s\x{3000}:：、.．·・\-—（(【\[])`
)

var (
	volumePattern = regexp.MustCompile(`^(?:第\s*` + numerals + `\s*[卷部集篇]|卷\s*` + numerals + `|(?i:part|book|volume)\s+(?:\d+|[IVXLCDM]+)\b)`)

	chapterPattern = regexp.MustCompile(`^(?:第\s*` + numerals + `\s*[章節节回話话幕]` +
		`|(?i:chapter)\s*\d+|(?i:chapter)\s+[IVXLCDM]+\b` +
		`|(?:序章|序言|序|楔子|引子|前言|後記|后记|尾聲|尾声|終章|终章|(?i:prologue|epilogue|preface|afterword))` + separator +
		`|番外)`)

	titlePattern  = regexp.MustCompile(`^(?:《(.+)》|(?:書名|书名|(?i:title))\s*[:：]\s*(.+))$`)
	authorPattern = regexp.MustCompile(`^(?:作\s*者|著者|(?i:author))\s*[:：]\s*(.+)$`)
	// tocTitlePattern matches the line that introduces a table of contents.
	tocTitlePattern = regexp.MustCompile(`^(?:目\s*[錄录次]|(?i:(?:table of )?contents))$`)
)

// maxHeadingLen is the longest line, in characters, that the built-in
// patterns accept as a heading. Longer lines are prose.
const maxHeadingLen = 40

// headerLines is how many non-empty lines at the start of the text are
// searched for the title and the author.
const headerLines = 10

// minTOCRun is the number of consecutive empty chapters that are taken to
// be a table of contents copied into the text, and dropped.
const minTOCRun = 3

type line struct {
	raw  string // without the line terminator
	text string // trimmed
}

type heading struct {
	index  int // line index
	title  string
	volume bool
}

// Parse splits text into chapters.
func Parse(text string, opts Options) (*Result, error) {
	isHeading, err := headingMatcher(opts.ChapterPattern)
	if err != nil {
		return nil, err
	}
	mode := opts.Paragraph
	if mode == "" {
		mode = ParagraphAuto
	}
	chunkSize := opts.ChunkSize
	if chunkSize <= 0 {
		chunkSize = DefaultChunkSize
	}

	lines := splitLines(text)
	if mode == ParagraphAuto {
		mode = detectParagraphMode(lines)
	}

	var headings []heading
	for i, l := range lines {
		if ok, volume := isHeading(l.text); ok {
			headings = append(headings, heading{index: i, title: l.text, volume: volume})
		}
	}

	res := &Result{}
	preambleEnd := len(lines)
	if len(headings) > 0 {
		preambleEnd = headings[0].index
	}
	res.Title, res.Author = findTitleAuthor(lines[:preambleEnd])

	if len(headings) == 0 {
		res.Chapters = splitParts(paragraphs(lines, mode), chunkSize, opts, res.Title)
		return res, nil
	}

	pre := paragraphs(lines[:preambleEnd], mode)

	type section struct {
		heading
		paras []string
	}
	var sections []section
	for i, h := range headings {
		end := len(lines)
		if i+1 < len(headings) {
			end = headings[i+1].index
		}
		sections = append(sections, section{h, paragraphs(lines[h.index+1:end], mode)})
	}

	// A run of headings without text is a table of contents copied into
	// the file. Its entries reappear later as the real headings, which is
	// how they are told apart from a real heading at the end of the run,
	// such as a volume heading followed by its first chapter.
	last := map[string]int{}
	for i, s := range sections {
		last[normalize(s.title)] = i
	}
	var kept []section
	for i := 0; i < len(sections); {
		j := i
		for j < len(sections) && len(sections[j].paras) == 0 {
			j++
		}
		if j == i {
			j++
		}
		prev := len(kept) - 1
		dropped := false
		for k := i; k < j; k++ {
			if j-i >= minTOCRun && len(sections[k].paras) == 0 && last[normalize(sections[k].title)] > k {
				dropped = true
				continue
			}
			kept = append(kept, sections[k])
		}
		// The text before the table of contents (the preamble or the
		// previous section) may end with its "目錄" line.
		if dropped {
			if prev < 0 {
				pre = dropTOCTitle(pre)
			} else {
				kept[prev].paras = dropTOCTitle(kept[prev].paras)
			}
		}
		i = j
	}

	if len(pre) > 0 {
		res.Chapters = append(res.Chapters, chapter(firstNonEmpty(res.Title, opts.FallbackTitle), 1, "", pre))
	}

	// Volumes nest the chapters only when the text has both kinds.
	hasVolume, hasChapter := false, false
	for _, s := range kept {
		hasVolume = hasVolume || s.volume
		hasChapter = hasChapter || !s.volume
	}
	nested := hasVolume && hasChapter

	for _, s := range kept {
		level, tag := 1, "h2"
		class := "chapter"
		if nested {
			if s.volume {
				tag, class = "h1", "volume"
			} else {
				level = 2
			}
		}
		h := fmt.Sprintf("<%s class=\"%s\">%s</%s>\n", tag, class, html.EscapeString(s.title), tag)
		res.Chapters = append(res.Chapters, chapter(s.title, level, h, s.paras))
	}
	return res, nil
}

// headingMatcher returns a function that reports whether a trimmed line is
// a heading, and whether it is a volume heading.
func headingMatcher(pattern string) (func(string) (ok, volume bool), error) {
	if pattern != "" {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid chapter pattern: %w", err)
		}
		return func(s string) (bool, bool) {
			return s != "" && re.MatchString(s), false
		}, nil
	}
	return func(s string) (bool, bool) {
		if s == "" || utf8.RuneCountInString(s) > maxHeadingLen {
			return false, false
		}
		// A heading does not end like a clause of prose.
		if r, _ := utf8.DecodeLastRuneInString(s); strings.ContainsRune("。，、；,;", r) {
			return false, false
		}
		if volumePattern.MatchString(s) {
			return true, true
		}
		return chapterPattern.MatchString(s), false
	}, nil
}

func splitLines(text string) []line {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	raw := strings.Split(text, "\n")
	lines := make([]line, len(raw))
	for i, r := range raw {
		lines[i] = line{raw: r, text: strings.TrimSpace(r)}
	}
	return lines
}

// detectParagraphMode chooses ParagraphBlank for hard-wrapped text: text
// whose blank-line separated blocks are several lines long and whose lines
// mostly end in the middle of a sentence.
func detectParagraphMode(lines []line) ParagraphMode {
	nonEmpty, blocks, sentenceEnds := 0, 0, 0
	inBlock := false
	for _, l := range lines {
		if l.text == "" {
			inBlock = false
			continue
		}
		nonEmpty++
		if !inBlock {
			blocks++
			inBlock = true
		}
		if endsSentence(l.text) {
			sentenceEnds++
		}
	}
	if blocks > 0 && nonEmpty >= 3*blocks && 2*sentenceEnds < nonEmpty {
		return ParagraphBlank
	}
	return ParagraphLine
}

func endsSentence(s string) bool {
	r, _ := utf8.DecodeLastRuneInString(s)
	return strings.ContainsRune("。！？!?.…」』\"”’'）)】》~～—:：", r)
}

// paragraphs groups lines into paragraph texts.
func paragraphs(lines []line, mode ParagraphMode) []string {
	var paras []string
	if mode == ParagraphLine {
		for _, l := range lines {
			if l.text != "" {
				paras = append(paras, l.text)
			}
		}
		return paras
	}
	var cur string
	flush := func() {
		if cur != "" {
			paras = append(paras, cur)
			cur = ""
		}
	}
	for _, l := range lines {
		switch {
		case l.text == "":
			flush()
		case cur == "":
			cur = l.text
		case isIndented(l.raw):
			flush()
			cur = l.text
		default:
			cur = joinLines(cur, l.text)
		}
	}
	flush()
	return paras
}

func isIndented(s string) bool {
	return strings.HasPrefix(s, "\u3000") || strings.HasPrefix(s, "  ") || strings.HasPrefix(s, "\t")
}

// joinLines joins two lines of a wrapped paragraph. CJK text has no spaces
// between words, so a line break between two wide characters is dropped.
func joinLines(a, b string) string {
	last, _ := utf8.DecodeLastRuneInString(a)
	first, _ := utf8.DecodeRuneInString(b)
	if isWide(last) && isWide(first) {
		return a + b
	}
	return a + " " + b
}

func isWide(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul) ||
		(r >= 0x3000 && r <= 0x303F) || // CJK symbols and punctuation
		(r >= 0xFF00 && r <= 0xFFEF) // full-width forms
}

func findTitleAuthor(lines []line) (title, author string) {
	n := 0
	for _, l := range lines {
		if l.text == "" {
			continue
		}
		if n++; n > headerLines {
			break
		}
		if m := titlePattern.FindStringSubmatch(l.text); m != nil && title == "" {
			title = strings.TrimSpace(firstNonEmpty(m[1], m[2]))
		} else if m := authorPattern.FindStringSubmatch(l.text); m != nil && author == "" {
			author = strings.TrimSpace(m[1])
		}
	}
	return title, author
}

// splitParts divides the paragraphs of a text without headings into parts
// of about chunkSize characters.
func splitParts(paras []string, chunkSize int, opts Options, title string) []book.Chapter {
	if len(paras) == 0 {
		return nil
	}
	var parts [][]string
	var cur []string
	size := 0
	for _, p := range paras {
		cur = append(cur, p)
		size += utf8.RuneCountInString(p)
		if size >= chunkSize {
			parts = append(parts, cur)
			cur, size = nil, 0
		}
	}
	if len(cur) > 0 {
		parts = append(parts, cur)
	}
	if len(parts) == 1 {
		return []book.Chapter{chapter(firstNonEmpty(title, opts.FallbackTitle), 1, "", parts[0])}
	}
	labels := book.LabelsFor(opts.Language)
	chapters := make([]book.Chapter, len(parts))
	for i, p := range parts {
		chapters[i] = chapter(labels.Part(i+1), 1, "", p)
	}
	return chapters
}

func chapter(title string, level int, headingHTML string, paras []string) book.Chapter {
	var sb strings.Builder
	sb.WriteString(headingHTML)
	for _, p := range paras {
		sb.WriteString("<p>")
		sb.WriteString(html.EscapeString(p))
		sb.WriteString("</p>\n")
	}
	return book.Chapter{Title: title, Level: level, Class: "txt", Body: sb.String()}
}

// dropTOCTitle removes a last paragraph such as "目錄".
func dropTOCTitle(paras []string) []string {
	if n := len(paras); n > 0 && tocTitlePattern.MatchString(paras[n-1]) {
		return paras[:n-1]
	}
	return paras
}

// normalize removes all white space, so that headings that differ only in
// spacing compare equal.
func normalize(s string) string {
	return strings.Join(strings.Fields(s), "")
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}
