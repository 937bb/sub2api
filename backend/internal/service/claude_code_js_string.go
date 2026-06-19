package service

import (
	"unicode/utf16"
	"unicode/utf8"
)

func jsStringIndexUTF8(s string, index int) ([]byte, bool) {
	if index < 0 {
		return nil, false
	}
	units := utf16.Encode([]rune(s))
	if index >= len(units) {
		return nil, false
	}
	r := rune(units[index])
	if utf16.IsSurrogate(r) {
		r = utf8.RuneError
	}
	buf := make([]byte, utf8.RuneLen(r))
	return buf[:utf8.EncodeRune(buf, r)], true
}
