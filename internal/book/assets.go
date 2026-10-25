package book

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AssetSet collects the image files of a book. Adding the same file twice
// returns the asset created the first time.
type AssetSet struct {
	list   []*Asset
	byPath map[string]*Asset
}

// List returns the assets in the order they were added.
func (s *AssetSet) List() []*Asset {
	return s.list
}

// AddImage reads an image file and stores it under images/ in the package.
// Only the EPUB core image types are accepted: JPEG, PNG, GIF, WebP and SVG.
func (s *AssetSet) AddImage(path string) (*Asset, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if a, ok := s.byPath[abs]; ok {
		return a, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	mediaType, ext := ImageType(path, data)
	if mediaType == "" {
		return nil, fmt.Errorf("%s: unsupported image format (want JPEG, PNG, GIF, WebP or SVG)", path)
	}
	a := &Asset{
		Href:      fmt.Sprintf("images/img-%04d%s", len(s.list)+1, ext),
		MediaType: mediaType,
		Data:      data,
	}
	if s.byPath == nil {
		s.byPath = map[string]*Asset{}
	}
	s.byPath[abs] = a
	s.list = append(s.list, a)
	return a, nil
}

// ImageType identifies an image by its content (SVG by its file extension)
// and returns its media type and preferred file extension. It returns empty
// strings for anything that is not an EPUB core image type.
func ImageType(name string, data []byte) (mediaType, ext string) {
	switch {
	case bytes.HasPrefix(data, []byte("\xFF\xD8\xFF")):
		return "image/jpeg", ".jpg"
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1A\n")):
		return "image/png", ".png"
	case bytes.HasPrefix(data, []byte("GIF87a")), bytes.HasPrefix(data, []byte("GIF89a")):
		return "image/gif", ".gif"
	case len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		return "image/webp", ".webp"
	case strings.EqualFold(filepath.Ext(name), ".svg"):
		return "image/svg+xml", ".svg"
	}
	return "", ""
}
