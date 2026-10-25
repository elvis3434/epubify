package textenc

import (
	"testing"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/encoding/unicode"
)

func encode(t *testing.T, enc encoding.Encoding, s string) []byte {
	t.Helper()
	b, err := enc.NewEncoder().Bytes([]byte(s))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

const (
	traditional = "第一章 開始\n他說這個世界上沒有什麼是不可能的。\n我們明天再來。\n"
	simplified  = "第一章 开始\n他说这个世界上没有什么是不可能的。\n我们明天再来。\n"
)

func TestDecodeDetect(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		want     string
		wantUsed string
	}{
		{"utf-8", []byte(traditional), traditional, "utf-8"},
		{"utf-8 bom", append([]byte("\xEF\xBB\xBF"), traditional...), traditional, "utf-8"},
		{"utf-16le bom", encode(t, unicode.UTF16(unicode.LittleEndian, unicode.UseBOM), traditional), traditional, "utf-16"},
		{"utf-16be bom", encode(t, unicode.UTF16(unicode.BigEndian, unicode.UseBOM), traditional), traditional, "utf-16"},
		{"big5", encode(t, traditionalchinese.Big5, traditional), traditional, "big5"},
		{"gbk simplified", encode(t, simplifiedchinese.GBK, simplified), simplified, "gb18030"},
		{"gbk traditional", encode(t, simplifiedchinese.GBK, traditional), traditional, "gb18030"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, used, err := Decode(tt.data, "")
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want || used != tt.wantUsed {
				t.Errorf("Decode() = %q, %q; want %q, %q", got, used, tt.want, tt.wantUsed)
			}
		})
	}
}

func TestDecodeNamed(t *testing.T) {
	got, used, err := Decode(encode(t, traditionalchinese.Big5, traditional), "BIG5")
	if err != nil {
		t.Fatal(err)
	}
	if got != traditional || used != "big5" {
		t.Errorf("Decode() = %q, %q", got, used)
	}

	sjis := []byte("\x82\xb1\x82\xf1\x82\xc9\x82\xbf\x82\xcd") // こんにちは in Shift_JIS
	if got, _, err := Decode(sjis, "shift_jis"); err != nil || got != "こんにちは" {
		t.Errorf("Decode(shift_jis) = %q, %v", got, err)
	}

	if _, _, err := Decode([]byte("x"), "no-such-encoding"); err == nil {
		t.Error("Decode() with an unknown encoding succeeded")
	}
}

func TestDecodeRemovesInvalidXMLCharacters(t *testing.T) {
	got, _, err := Decode([]byte("a\x00b\x1Fc\td\r\ne\x7F"), "")
	if err != nil {
		t.Fatal(err)
	}
	if want := "abc\td\r\ne"; got != want {
		t.Errorf("Decode() = %q; want %q", got, want)
	}
}
