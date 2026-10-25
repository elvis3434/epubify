// Command epubify converts .txt and .md files into an EPUB 3 book.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/elvis3434/epubify"
)

// version is set at build time with -ldflags "-X main.version=v1.2.3".
var version = ""

const usage = `Usage: epubify [options] <file or directory>...

Convert .txt and .md files into an EPUB 3 book. Several inputs become one
book; a directory adds its .txt and .md files in natural order.

Options:
  -o, --output FILE        output file (default: first input with .epub)
  -t, --title TEXT         book title
  -a, --author NAME        author; repeat the option for several authors
  -l, --lang TAG           language tag (default: zh-Hant)
      --cover FILE         cover image (JPEG, PNG, GIF, WebP or SVG)
      --vertical           vertical writing, right-to-left page order
      --css FILE           extra stylesheet
      --encoding NAME      input encoding: auto, utf-8, big5, gbk, ... (default: auto)
      --chapter-regex RE   regular expression matching chapter headings in .txt files
      --paragraph MODE     paragraphs in .txt files: auto, line or blank (default: auto)
      --publisher TEXT     publisher
      --description TEXT   description
      --id TEXT            unique identifier (default: derived from title and authors)
  -q, --quiet              do not print notes and warnings
  -v, --version            print the version and exit
  -h, --help               print this help and exit

Examples:
  epubify novel.txt
  epubify -t "書名" -a "作者" --cover cover.jpg novel.txt
  epubify --vertical --encoding big5 novel.txt
  epubify -o notes.epub notes/
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	var (
		opts               epubify.Options
		output, cover, css string
		quiet, showVersion bool
	)
	fs := flag.NewFlagSet("epubify", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}
	str := func(p *string, names ...string) {
		for _, n := range names {
			fs.StringVar(p, n, "", "")
		}
	}
	boolean := func(p *bool, names ...string) {
		for _, n := range names {
			fs.BoolVar(p, n, false, "")
		}
	}
	str(&output, "o", "output")
	str(&opts.Title, "t", "title")
	str(&opts.Language, "l", "lang")
	str(&cover, "cover")
	str(&css, "css")
	str(&opts.Encoding, "encoding")
	str(&opts.ChapterPattern, "chapter-regex")
	str(&opts.Paragraph, "paragraph")
	str(&opts.Publisher, "publisher")
	str(&opts.Description, "description")
	str(&opts.Identifier, "id")
	for _, n := range []string{"a", "author"} {
		fs.Func(n, "", func(s string) error {
			opts.Authors = append(opts.Authors, s)
			return nil
		})
	}
	boolean(&opts.Vertical, "vertical")
	boolean(&quiet, "q", "quiet")
	boolean(&showVersion, "v", "version")

	inputs, err := parseInterspersed(fs, args)
	if errors.Is(err, flag.ErrHelp) {
		fmt.Fprint(stdout, usage)
		return 0
	}
	if err != nil {
		fmt.Fprintf(stderr, "epubify: %v\n\n%s", err, usage)
		return 2
	}
	if showVersion {
		fmt.Fprintln(stdout, "epubify", versionString())
		return 0
	}
	if len(inputs) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}

	opts.Cover = cover
	if css != "" {
		data, err := os.ReadFile(css)
		if err != nil {
			fmt.Fprintf(stderr, "epubify: %v\n", err)
			return 1
		}
		opts.CSS = string(data)
	}
	if !quiet {
		opts.Logf = func(format string, args ...any) {
			fmt.Fprintf(stderr, "epubify: "+format+"\n", args...)
		}
	}
	// Reproducible builds: https://reproducible-builds.org/specs/source-date-epoch/
	if s := os.Getenv("SOURCE_DATE_EPOCH"); s != "" {
		sec, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			fmt.Fprintf(stderr, "epubify: invalid SOURCE_DATE_EPOCH %q\n", s)
			return 1
		}
		opts.Modified = time.Unix(sec, 0)
	}
	if output == "" {
		output = defaultOutput(inputs[0])
	}

	sum, err := epubify.ConvertFile(output, inputs, opts)
	if err != nil {
		fmt.Fprintf(stderr, "epubify: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "%s: %q, %d chapters, %d images\n", output, sum.Title, sum.Chapters, sum.Images)
	return 0
}

// parseInterspersed parses flags that appear before, between or after the
// positional arguments. Everything after "--" is positional.
func parseInterspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		rest := fs.Args()
		// fs.Parse stops at the first non-flag or after "--".
		consumed := len(args) - len(rest)
		if consumed > 0 && args[consumed-1] == "--" {
			return append(positional, rest...), nil
		}
		if len(rest) == 0 {
			return positional, nil
		}
		positional = append(positional, rest[0])
		args = rest[1:]
	}
}

// defaultOutput names the book after the first input: a file's book is
// written next to it, a directory's book into the current directory.
func defaultOutput(input string) string {
	if strings.ContainsAny(input, "*?[") {
		if _, err := os.Stat(input); err != nil {
			if matches, _ := filepath.Glob(input); len(matches) > 0 {
				input = matches[0]
			}
		}
	}
	clean := filepath.Clean(input)
	if info, err := os.Stat(clean); err == nil && info.IsDir() {
		if abs, err := filepath.Abs(clean); err == nil {
			return filepath.Base(abs) + ".epub"
		}
	}
	return strings.TrimSuffix(clean, filepath.Ext(clean)) + ".epub"
}

func versionString() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}
