package main

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestParseInterspersed(t *testing.T) {
	tests := []struct {
		args      []string
		wantPos   []string
		wantTitle string
	}{
		{[]string{"a.txt", "-t", "T", "b.md"}, []string{"a.txt", "b.md"}, "T"},
		{[]string{"--title=T", "a.txt"}, []string{"a.txt"}, "T"},
		{[]string{"a.txt", "--", "-t", "b.md"}, []string{"a.txt", "-t", "b.md"}, ""},
		{nil, nil, ""},
	}
	for _, tt := range tests {
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		title := fs.String("t", "", "")
		fs.StringVar(title, "title", "", "")
		pos, err := parseInterspersed(fs, tt.args)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(pos, tt.wantPos) || *title != tt.wantTitle {
			t.Errorf("parse(%q) = %q, title %q", tt.args, pos, *title)
		}
	}
}

func TestDefaultOutput(t *testing.T) {
	dir := t.TempDir()
	if got, want := defaultOutput(filepath.Join(dir, "novel.txt")), filepath.Join(dir, "novel.epub"); got != want {
		t.Errorf("file: %q; want %q", got, want)
	}
	if got, want := defaultOutput(dir+string(filepath.Separator)), filepath.Base(dir)+".epub"; got != want {
		t.Errorf("directory: %q; want %q", got, want)
	}
}

func runCLI(args ...string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer
	code = run(args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestRun(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1761350400")
	dir := t.TempDir()
	css := filepath.Join(dir, "extra.css")
	if err := os.WriteFile(css, []byte("p { color: red; }"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "book.epub")
	code, stdout, stderr := runCLI("../../testdata/notes.md", "-o", out, "-a", "甲", "--author", "乙", "--css", css, "-q")
	if code != 0 {
		t.Fatalf("exit code %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, "4 chapters, 2 images") || stderr != "" {
		t.Errorf("stdout %q, stderr %q", stdout, stderr)
	}
	first, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	// SOURCE_DATE_EPOCH makes the output reproducible.
	if code, _, _ := runCLI("-q", "-o", out, "-a", "甲", "-a", "乙", "--css", css, "../../testdata/notes.md"); code != 0 {
		t.Fatal("second run failed")
	}
	if second, _ := os.ReadFile(out); !bytes.Equal(first, second) {
		t.Error("output is not reproducible")
	}

	// Notes go to stderr unless -q is given.
	if _, _, stderr := runCLI("-o", out, "../../testdata/notes.md"); !strings.Contains(stderr, "remote images") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestRunUsage(t *testing.T) {
	if code, stdout, _ := runCLI("-h"); code != 0 || !strings.HasPrefix(stdout, "Usage:") {
		t.Errorf("-h: code %d, stdout %q", code, stdout)
	}
	if code, stdout, _ := runCLI("--version"); code != 0 || !strings.HasPrefix(stdout, "epubify ") {
		t.Errorf("--version: code %d, stdout %q", code, stdout)
	}
	if code, _, stderr := runCLI(); code != 2 || !strings.Contains(stderr, "Usage:") {
		t.Errorf("no args: code %d", code)
	}
	if code, _, stderr := runCLI("--bogus", "a.txt"); code != 2 || !strings.Contains(stderr, "bogus") {
		t.Errorf("unknown flag: code %d, stderr %q", code, stderr)
	}
	if code, _, stderr := runCLI(filepath.Join(t.TempDir(), "none.txt")); code != 1 || !strings.HasPrefix(stderr, "epubify: ") {
		t.Errorf("missing file: code %d, stderr %q", code, stderr)
	}
}
