package epubify

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
)

var fixed = time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC)

func convert(t *testing.T, inputs []string, opts Options) (Summary, map[string]string, []byte) {
	t.Helper()
	if opts.Modified.IsZero() {
		opts.Modified = fixed
	}
	var buf bytes.Buffer
	sum, err := Convert(&buf, inputs, opts)
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(rc)
		rc.Close()
		files[f.Name] = string(b)
	}
	return sum, files, buf.Bytes()
}

func TestConvertText(t *testing.T) {
	sum, files, _ := convert(t, []string{"testdata/novel.txt"}, Options{})
	if sum.Title != "山城夜雨" || sum.Chapters != 6 || sum.Images != 0 {
		t.Errorf("summary = %+v", sum)
	}
	opf := files["OEBPS/content.opf"]
	for _, want := range []string{
		"<dc:title>山城夜雨</dc:title>",
		`<dc:creator id="creator-1">林小雨</dc:creator>`,
		"<dc:language>zh-Hant</dc:language>",
		"<meta property=\"dcterms:modified\">2026-10-25T00:00:00Z</meta>",
	} {
		if !strings.Contains(opf, want) {
			t.Errorf("content.opf lacks %s", want)
		}
	}
	if !regexp.MustCompile(`urn:uuid:[0-9a-f]{8}-[0-9a-f]{4}-5[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}`).MatchString(opf) {
		t.Errorf("content.opf has no version 5 UUID")
	}
}

func TestConvertOptionsOverride(t *testing.T) {
	_, files, _ := convert(t, []string{"testdata/notes.md"}, Options{
		Title:      "覆寫",
		Authors:    []string{"新作者"},
		Language:   "en-us",
		Identifier: "urn:isbn:9789570000000",
		Vertical:   true,
	})
	opf := files["OEBPS/content.opf"]
	for _, want := range []string{
		"<dc:title>覆寫</dc:title>",
		`<dc:creator id="creator-1">新作者</dc:creator>`,
		"<dc:language>en-US</dc:language>",
		`<dc:identifier id="book-id">urn:isbn:9789570000000</dc:identifier>`,
		`page-progression-direction="rtl"`,
	} {
		if !strings.Contains(opf, want) {
			t.Errorf("content.opf lacks %s\n%s", want, opf)
		}
	}
	if strings.Contains(opf, "王大明") {
		t.Error("front matter author was not overridden")
	}
}

func TestConvertMarkdownFrontMatter(t *testing.T) {
	var logs []string
	sum, files, _ := convert(t, []string{"testdata/notes.md"}, Options{
		Logf: func(format string, args ...any) { logs = append(logs, format) },
	})
	if sum.Title != "Markdown 範例筆記" || sum.Chapters != 4 || sum.Images != 2 {
		t.Errorf("summary = %+v", sum)
	}
	if _, ok := files["OEBPS/cover.xhtml"]; !ok {
		t.Error("no cover page from the front matter")
	}
	opf := files["OEBPS/content.opf"]
	if !strings.Contains(opf, "<dc:date>2026-10-25</dc:date>") || !strings.Contains(opf, `<dc:creator id="creator-2">陳小華</dc:creator>`) {
		t.Errorf("content.opf:\n%s", opf)
	}
	if len(logs) != 2 {
		t.Errorf("logs = %q; want the remote image and the local link", logs)
	}
}

func TestConvertSeveralInputs(t *testing.T) {
	sum, files, _ := convert(t, []string{"testdata/novel.txt", "testdata/notes.md"}, Options{})
	if sum.Title != "山城夜雨" || sum.Chapters != 10 {
		t.Errorf("summary = %+v", sum)
	}
	// Footnote links in the Markdown chapters point at the right files.
	if !strings.Contains(files["OEBPS/chapter-0009.xhtml"], `href="chapter-0009.xhtml#fn-1"`) {
		t.Errorf("chapter 9:\n%s", files["OEBPS/chapter-0009.xhtml"])
	}
}

func TestConvertDirectoryAndGlob(t *testing.T) {
	dir := t.TempDir()
	for name, text := range map[string]string{
		"10.md": "# Ten\n", "2.md": "# Two\n", "1.txt": "第一章\n一。\n", "skip.epub": "x", ".hidden.md": "# Hidden\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	_, files, _ := convert(t, []string{dir}, Options{})
	nav := files["OEBPS/nav.xhtml"]
	if i, j, k := strings.Index(nav, "第一章"), strings.Index(nav, "Two"), strings.Index(nav, "Ten"); i < 0 || i > j || j > k {
		t.Errorf("chapters not in natural order:\n%s", nav)
	}
	if strings.Contains(nav, "Hidden") {
		t.Error("hidden file included")
	}
	if !strings.Contains(files["OEBPS/content.opf"], "<dc:title>"+filepath.Base(dir)+"</dc:title>") {
		t.Error("book not named after the directory")
	}

	sum, _, _ := convert(t, []string{filepath.Join(dir, "*.md")}, Options{})
	if sum.Chapters != 2 {
		t.Errorf("glob gave %d chapters; want 2", sum.Chapters)
	}
}

func TestConvertErrors(t *testing.T) {
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty.txt")
	other := filepath.Join(dir, "a.html")
	for _, f := range []string{empty, other} {
		if err := os.WriteFile(f, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tests := []struct {
		name   string
		inputs []string
		opts   Options
	}{
		{"no inputs", nil, Options{}},
		{"missing file", []string{filepath.Join(dir, "none.txt")}, Options{}},
		{"unsupported type", []string{other}, Options{}},
		{"empty text", []string{empty}, Options{}},
		{"no glob match", []string{filepath.Join(dir, "*.md")}, Options{}},
		{"invalid language", []string{"testdata/novel.txt"}, Options{Language: "not a tag"}},
		{"missing cover", []string{"testdata/novel.txt"}, Options{Cover: filepath.Join(dir, "none.jpg")}},
		{"unsupported cover", []string{"testdata/novel.txt"}, Options{Cover: "testdata/novel.txt"}},
		{"invalid paragraph mode", []string{"testdata/novel.txt"}, Options{Paragraph: "words"}},
		{"unknown encoding", []string{"testdata/novel.txt"}, Options{Encoding: "klingon"}},
	}
	for _, tt := range tests {
		if _, err := Convert(io.Discard, tt.inputs, tt.opts); err == nil {
			t.Errorf("%s: Convert() succeeded", tt.name)
		}
	}
}

func TestConvertIsDeterministic(t *testing.T) {
	_, _, a := convert(t, []string{"testdata/notes.md"}, Options{})
	_, _, b := convert(t, []string{"testdata/notes.md"}, Options{})
	if !bytes.Equal(a, b) {
		t.Error("two conversions of the same input differ")
	}
}

func TestConvertFile(t *testing.T) {
	out := filepath.Join(t.TempDir(), "book.epub")
	if _, err := ConvertFile(out, []string{"testdata/novel.txt"}, Options{}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(out)
	if err != nil || info.Size() == 0 {
		t.Fatalf("output: %v, %v", info, err)
	}
	// A failed conversion leaves the existing file and no temporary file.
	if _, err := ConvertFile(out, []string{"testdata/none.txt"}, Options{}); err == nil {
		t.Fatal("ConvertFile() succeeded")
	}
	entries, _ := os.ReadDir(filepath.Dir(out))
	if len(entries) != 1 {
		t.Errorf("directory has %d entries; want 1", len(entries))
	}
}

func TestNaturalCompare(t *testing.T) {
	got := []string{"10.md", "2.md", "1.md", "a10", "a2", "a02", "a02b", "b", "a"}
	slices.SortFunc(got, naturalCompare)
	want := []string{"1.md", "2.md", "10.md", "a", "a02", "a2", "a02b", "a10", "b"}
	if !slices.Equal(got, want) {
		t.Errorf("sorted = %q; want %q", got, want)
	}
}

func TestNameUUID(t *testing.T) {
	a := nameUUID("書", []string{"甲"})
	if a != nameUUID("書", []string{"甲"}) {
		t.Error("nameUUID is not stable")
	}
	if a == nameUUID("書", []string{"乙"}) || a == nameUUID("書甲", nil) {
		t.Error("nameUUID collides")
	}
}

// TestEPUBCheck validates the books with EPUBCheck when EPUBCHECK_JAR
// points at epubcheck.jar.
func TestEPUBCheck(t *testing.T) {
	jar := os.Getenv("EPUBCHECK_JAR")
	if jar == "" {
		t.Skip("EPUBCHECK_JAR is not set")
	}
	dir := t.TempDir()
	books := map[string]struct {
		inputs []string
		opts   Options
	}{
		"novel":     {[]string{"testdata/novel.txt"}, Options{}},
		"vertical":  {[]string{"testdata/novel.txt"}, Options{Vertical: true, Cover: "testdata/images/cover.jpg"}},
		"notes":     {[]string{"testdata/notes.md"}, Options{}},
		"combined":  {[]string{"testdata/novel.txt", "testdata/notes.md"}, Options{Language: "en"}},
		"directory": {[]string{"testdata"}, Options{}},
	}
	for name, b := range books {
		t.Run(name, func(t *testing.T) {
			out := filepath.Join(dir, name+".epub")
			if _, err := ConvertFile(out, b.inputs, b.opts); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("java", "-jar", jar, "--failonwarnings", out)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Errorf("epubcheck: %v\n%s", err, output)
			}
		})
	}
}
