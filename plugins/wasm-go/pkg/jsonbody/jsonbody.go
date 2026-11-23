// Package jsonbody inspects raw JSON request bodies structurally, without
// decoding them into Go values.
package jsonbody

import (
	"bytes"
	"errors"
	"unicode/utf16"
	"unicode/utf8"
)

// ErrMalformedJSON reports that the input is not a complete JSON document, so
// nothing can be concluded about its members. Callers should treat it as "no
// duplicate found" and leave the body to the plugin's existing validation.
var ErrMalformedJSON = errors.New("jsonbody: malformed JSON document")

// DuplicateTopLevelMember reports whether the top-level JSON object in body
// declares a member named key more than once.
//
// gjson reads the first occurrence of a duplicated member while sjson rewrites
// and mainstream upstream parsers (encoding/json in Go, Python and Node) settle
// on the last one, so the gateway and the upstream can disagree about which
// model a request refers to. Plugins call this before making model-based
// routing, authorization or mapping decisions to normalize that ambiguity.
//
// The body is scanned once instead of being decoded: only container nesting,
// string literals and escapes are tracked, and the scan stops at the second
// occurrence. Counting is therefore structural - a key spelling inside a string
// value or inside a nested object is never mistaken for a top-level member.
//
// The scan does not validate JSON grammar, so a defect such as a trailing comma
// is left to the caller's existing body validation. Truncated or unbalanced
// input yields ErrMalformedJSON rather than a verdict.
func DuplicateTopLevelMember(body []byte, key string) (bool, error) {
	depth := 0
	topLevelObject := false
	// memberNext reports whether a string literal starting at the current
	// position names a member rather than holding a value.
	memberNext := false
	occurrences := 0

	for i := 0; i < len(body); i++ {
		switch body[i] {
		case '"':
			literal, next, err := scanStringLiteral(body, i)
			if err != nil {
				return false, err
			}
			if memberNext && depth == 1 && literalNames(literal, key) {
				occurrences++
				if occurrences > 1 {
					return true, nil
				}
			}
			memberNext = false
			i = next - 1
		case '{':
			depth++
			if depth == 1 {
				topLevelObject = true
			}
			memberNext = true
		case '[':
			depth++
			memberNext = false
		case '}', ']':
			depth--
			if depth < 0 {
				return false, ErrMalformedJSON
			}
			memberNext = false
		case ':':
			memberNext = false
		case ',':
			memberNext = depth == 1 && topLevelObject
		}
	}
	if depth != 0 {
		return false, ErrMalformedJSON
	}
	return false, nil
}

// DuplicateMemberMessage renders the reject body plugins return for a request
// body carrying a duplicated top-level member, keeping the wording identical
// across the AI plugin family.
func DuplicateMemberMessage(key string) string {
	return `duplicate "` + key + `" member in request body`
}

// scanStringLiteral returns the undecoded content of the JSON string literal
// opening at body[open] together with the index just past its closing quote.
func scanStringLiteral(body []byte, open int) (literal []byte, next int, err error) {
	for i := open + 1; i < len(body); i++ {
		switch body[i] {
		case '\\':
			i++
			if i == len(body) {
				return nil, 0, ErrMalformedJSON
			}
		case '"':
			return body[open+1 : i], i + 1, nil
		}
	}
	return nil, 0, ErrMalformedJSON
}

// literalNames reports whether an undecoded string literal spells key. The
// escape-free form is compared directly, which keeps the common case free of
// allocation and decoding.
func literalNames(literal []byte, key string) bool {
	if bytes.IndexByte(literal, '\\') < 0 {
		return string(literal) == key
	}
	var stack [64]byte
	decoded, ok := unescapeLiteral(literal, stack[:0])
	return ok && string(decoded) == key
}

// unescapeLiteral appends the decoded form of a string literal to dst. It
// reports false when the literal holds an invalid escape sequence, in which
// case the caller cannot match it against a plain key either.
func unescapeLiteral(literal []byte, dst []byte) ([]byte, bool) {
	for i := 0; i < len(literal); i++ {
		if literal[i] != '\\' {
			dst = append(dst, literal[i])
			continue
		}
		i++
		if i == len(literal) {
			return dst, false
		}
		switch literal[i] {
		case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
			dst = append(dst, unescapeSimple(literal[i]))
		case 'u':
			r, width, ok := decodeEscape(literal, i+1)
			if !ok {
				return dst, false
			}
			dst = utf8.AppendRune(dst, r)
			i += width
		default:
			return dst, false
		}
	}
	return dst, true
}

func unescapeSimple(c byte) byte {
	switch c {
	case 'b':
		return '\b'
	case 'f':
		return '\f'
	case 'n':
		return '\n'
	case 'r':
		return '\r'
	case 't':
		return '\t'
	default:
		return c
	}
}

// decodeEscape reads the four hex digits of a \u escape at hex, joining a
// surrogate pair when present. It returns the decoded rune and how many bytes
// of literal were consumed past the digits' starting offset.
func decodeEscape(literal []byte, hex int) (rune, int, bool) {
	r, ok := parseHex4(literal, hex)
	if !ok {
		return 0, 0, false
	}
	consumed := 4
	if utf16.IsSurrogate(r) && len(literal) >= hex+10 &&
		literal[hex+4] == '\\' && literal[hex+5] == 'u' {
		if low, ok := parseHex4(literal, hex+6); ok {
			if joined := utf16.DecodeRune(r, low); joined != utf8.RuneError {
				return joined, 10, true
			}
		}
	}
	return r, consumed, true
}

func parseHex4(literal []byte, offset int) (rune, bool) {
	if len(literal) < offset+4 {
		return 0, false
	}
	var r rune
	for _, c := range literal[offset : offset+4] {
		r <<= 4
		switch {
		case c >= '0' && c <= '9':
			r |= rune(c - '0')
		case c >= 'a' && c <= 'f':
			r |= rune(c-'a') + 10
		case c >= 'A' && c <= 'F':
			r |= rune(c-'A') + 10
		default:
			return 0, false
		}
	}
	return r, true
}
