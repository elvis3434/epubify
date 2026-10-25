// Package textenc decodes text files into UTF-8.
//
// UTF-8 and UTF-16 with a byte order mark are recognized exactly. Other
// text is assumed to be Chinese and decoded as Big5 or GB18030 (a superset
// of GBK), whichever produces more common Chinese characters.
package textenc

import (
	"bytes"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/htmlindex"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/encoding/unicode"
)

// Decode converts data to a UTF-8 string. name is an encoding label such
// as "big5", "gbk" or "shift_jis" (any WHATWG label); "" or "auto" detects
// the encoding. Decode returns the name of the encoding it used. Characters
// that are not allowed in XML are removed from the result.
func Decode(data []byte, name string) (text, used string, err error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name != "" && name != "auto" {
		enc, err := htmlindex.Get(name)
		if err != nil {
			return "", "", fmt.Errorf("unknown encoding %q", name)
		}
		s, err := decodeWith(enc, stripBOM(data))
		return clean(s), name, err
	}

	switch {
	case bytes.HasPrefix(data, []byte("\xEF\xBB\xBF")):
		return clean(string(data[3:])), "utf-8", nil
	case bytes.HasPrefix(data, []byte("\xFF\xFE")), bytes.HasPrefix(data, []byte("\xFE\xFF")):
		s, err := decodeWith(unicode.UTF16(unicode.BigEndian, unicode.ExpectBOM), data)
		return clean(s), "utf-16", err
	case utf8.Valid(data):
		return clean(string(data)), "utf-8", nil
	}

	best, bestName, bestScore := "", "", 0
	for _, c := range []struct {
		name string
		enc  encoding.Encoding
	}{
		{"big5", traditionalchinese.Big5},
		{"gb18030", simplifiedchinese.GB18030},
	} {
		s, err := decodeWith(c.enc, data)
		if err != nil {
			continue
		}
		if score := chineseScore(s); bestName == "" || score > bestScore {
			best, bestName, bestScore = s, c.name, score
		}
	}
	if bestName == "" {
		return "", "", fmt.Errorf("cannot detect the text encoding; specify one, e.g. big5 or gbk")
	}
	return clean(best), bestName, nil
}

// stripBOM removes a UTF-8 byte order mark, which some editors add even
// when saving in another encoding is not possible.
func stripBOM(data []byte) []byte {
	return bytes.TrimPrefix(data, []byte("\xEF\xBB\xBF"))
}

func decodeWith(enc encoding.Encoding, data []byte) (string, error) {
	out, err := enc.NewDecoder().Bytes(data)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// common holds frequent Chinese characters in both traditional and
// simplified forms. Text decoded with the wrong encoding is mostly rare
// characters, so it scores low.
const common = "的一是不了人我在有他這这中大來来上個个們们到說说你地也就那要會会着著" +
	"子出時时年得下自之過过以後后家生可對对裡里去她心道麼么看天好都然沒没" +
	"起還还發发成事只作當当想樣样經经知於于"

// chineseScore counts common characters and penalizes replacement
// characters, which mark bytes the decoder could not map.
func chineseScore(s string) int {
	score := 0
	for _, r := range s {
		switch {
		case r == utf8.RuneError:
			score -= 5
		case strings.ContainsRune(common, r):
			score++
		}
	}
	return score
}

// clean removes a leading byte order mark and the characters that are not
// allowed in XML 1.0: control characters other than tab and line breaks,
// and U+FFFE/U+FFFF.
func clean(s string) string {
	s = strings.TrimPrefix(s, "\uFEFF")
	keep := func(r rune) bool {
		return r == '\t' || r == '\n' || r == '\r' || (r >= 0x20 && r != 0x7F && r != 0xFFFE && r != 0xFFFF)
	}
	for _, r := range s {
		if !keep(r) {
			return strings.Map(func(r rune) rune {
				if keep(r) {
					return r
				}
				return -1
			}, s)
		}
	}
	return s
}
