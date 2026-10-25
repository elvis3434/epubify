package markdown

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v2"
)

// FrontMatter is the book metadata from a YAML block at the start of a
// Markdown file:
//
//	---
//	title: 書名
//	author: 作者
//	lang: zh-Hant
//	cover: images/cover.jpg
//	---
type FrontMatter struct {
	Title       string
	Authors     []string
	Language    string
	Cover       string // path relative to the Markdown file
	Description string
	Publisher   string
	Date        string
	Identifier  string
}

var datePattern = regexp.MustCompile(`^\d{4}(?:-\d{2}(?:-\d{2}(?:T[0-9:.]+(?:Z|[+-]\d{2}:\d{2})?)?)?)?$`)

// splitFrontMatter separates the front matter from the Markdown body. A
// front matter block starts with a "---" line at the very beginning and
// ends with a "---" or "..." line.
func splitFrontMatter(src []byte) (FrontMatter, []byte, error) {
	var fm FrontMatter
	lines := bytes.SplitAfter(src, []byte("\n"))
	if len(lines) == 0 || trimLine(lines[0]) != "---" {
		return fm, src, nil
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if t := trimLine(lines[i]); t == "---" || t == "..." {
			end = i
			break
		}
	}
	if end < 0 {
		return fm, src, nil
	}
	body := bytes.Join(lines[end+1:], nil)

	var m map[string]any
	if err := yaml.Unmarshal(bytes.Join(lines[1:end], nil), &m); err != nil {
		return fm, nil, fmt.Errorf("front matter: %w", err)
	}
	for k, v := range m {
		switch strings.ToLower(k) {
		case "title":
			fm.Title = scalar(v)
		case "author", "authors", "creator":
			fm.Authors = list(v)
		case "lang", "language":
			fm.Language = scalar(v)
		case "cover", "cover-image", "cover_image":
			fm.Cover = scalar(v)
		case "description", "summary":
			fm.Description = scalar(v)
		case "publisher":
			fm.Publisher = scalar(v)
		case "date":
			fm.Date = scalar(v)
		case "identifier", "id", "isbn":
			fm.Identifier = scalar(v)
		}
	}
	if fm.Date != "" && !datePattern.MatchString(fm.Date) {
		return fm, nil, fmt.Errorf("front matter: date %q is not in YYYY-MM-DD form", fm.Date)
	}
	return fm, body, nil
}

func trimLine(b []byte) string {
	return strings.TrimRight(string(b), " \t\r\n")
}

func scalar(v any) string {
	switch v := v.(type) {
	case nil:
		return ""
	case time.Time:
		return v.Format("2006-01-02")
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

func list(v any) []string {
	var out []string
	if items, ok := v.([]any); ok {
		for _, item := range items {
			if s := scalar(item); s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	if s := scalar(v); s != "" {
		out = append(out, s)
	}
	return out
}
