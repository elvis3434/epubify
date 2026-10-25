package book

import (
	"os"
	"path/filepath"
	"testing"
)

func TestImageType(t *testing.T) {
	tests := []struct {
		name, data, mediaType, ext string
	}{
		{"a.jpeg", "\xFF\xD8\xFF\xE0", "image/jpeg", ".jpg"},
		{"a.bin", "\x89PNG\r\n\x1A\n", "image/png", ".png"},
		{"a.gif", "GIF89a", "image/gif", ".gif"},
		{"a.webp", "RIFF\x00\x00\x00\x00WEBPVP8 ", "image/webp", ".webp"},
		{"a.SVG", "<svg/>", "image/svg+xml", ".svg"},
		{"a.png", "not an image", "", ""},
		{"a.bmp", "BM", "", ""},
	}
	for _, tt := range tests {
		mt, ext := ImageType(tt.name, []byte(tt.data))
		if mt != tt.mediaType || ext != tt.ext {
			t.Errorf("ImageType(%q) = %q, %q; want %q, %q", tt.name, mt, ext, tt.mediaType, tt.ext)
		}
	}
}

func TestAssetSet(t *testing.T) {
	dir := t.TempDir()
	png := filepath.Join(dir, "a.png")
	gif := filepath.Join(dir, "b.gif")
	bad := filepath.Join(dir, "c.png")
	for name, data := range map[string]string{png: "\x89PNG\r\n\x1A\n", gif: "GIF87a", bad: "text"} {
		if err := os.WriteFile(name, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var s AssetSet
	a, err := s.AddImage(png)
	if err != nil {
		t.Fatal(err)
	}
	if again, _ := s.AddImage(filepath.Join(dir, ".", "a.png")); again != a {
		t.Error("the same file was added twice")
	}
	b, err := s.AddImage(gif)
	if err != nil {
		t.Fatal(err)
	}
	if a.Href != "images/img-0001.png" || b.Href != "images/img-0002.gif" || len(s.List()) != 2 {
		t.Errorf("assets = %q, %q, %d", a.Href, b.Href, len(s.List()))
	}
	if _, err := s.AddImage(bad); err == nil {
		t.Error("AddImage accepted a file that is not an image")
	}
	if _, err := s.AddImage(filepath.Join(dir, "none.png")); err == nil {
		t.Error("AddImage accepted a missing file")
	}
}

func TestLabelsFor(t *testing.T) {
	tests := []struct{ lang, contents, part string }{
		{"zh-Hant", "目錄", "第 2 部分"},
		{"zh-TW", "目錄", "第 2 部分"},
		{"zh-Hans", "目录", "第 2 部分"},
		{"zh-CN", "目录", "第 2 部分"},
		{"ja", "目次", "第2部"},
		{"en", "Contents", "Part 2"},
		{"", "Contents", "Part 2"},
	}
	for _, tt := range tests {
		l := LabelsFor(tt.lang)
		if l.Contents != tt.contents || l.Part(2) != tt.part {
			t.Errorf("LabelsFor(%q) = %q, %q", tt.lang, l.Contents, l.Part(2))
		}
	}
}
