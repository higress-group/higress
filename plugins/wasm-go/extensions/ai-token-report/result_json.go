package main

import "encoding/json"

// resultJSON recognizes the billing fields while validating one complete JSON
// object across callbacks. String values and unknown keys may be arbitrarily
// large. Only structural state and keys short enough to be relevant are kept.
type resultJSON struct {
	search                            bool
	stack                             []resultFrame
	started, invalid, found, hasError bool

	inString, readingKey, imageString, nonempty, escape bool
	unicodeDigits                                       int
	// The longest relevant key is b64_json (8 characters, up to 6 bytes per escape).
	key     [8 * 6]byte
	keySize int
	literal string
	number  numberState
}

type resultFrame struct {
	role   resultRole
	expect jsonExpectation
	field  string
}

type resultRole uint8

const (
	irrelevant resultRole = iota
	rootObject
	imageArray
	imageObject
)

type jsonExpectation uint8

const (
	firstKey jsonExpectation = iota // empty object allowed
	nextKey
	colon
	objectValue
	objectEnd
	firstElement // empty array allowed
	nextElement
	arrayEnd
)

func (p *resultJSON) billable() bool {
	return p.started && !p.invalid && len(p.stack) == 0 && p.found && !p.hasError
}

func (p *resultJSON) feed(chunk []byte) {
	for i := 0; i < len(chunk) && !p.invalid; i++ {
		c := chunk[i]
		switch {
		case p.inString:
			p.stringByte(c)
		case p.literal != "":
			if c != p.literal[0] {
				p.invalid = true
				return
			}
			p.literal = p.literal[1:]
		case p.number != noNumber:
			if !p.number.consume(c) {
				p.invalid = !p.number.complete()
				p.number = noNumber
				i-- // The delimiter belongs to the containing array or object.
			}
		default:
			p.structuralByte(c)
		}
	}
}

func (p *resultJSON) structuralByte(c byte) {
	switch c {
	case ' ', '\t', '\r', '\n':
		return
	}
	if len(p.stack) == 0 {
		if p.started || c != '{' {
			p.invalid = true
			return
		}
		p.started = true
		p.stack = append(p.stack, resultFrame{role: rootObject, expect: firstKey})
		return
	}

	f := &p.stack[len(p.stack)-1]
	switch f.expect {
	case firstKey, nextKey:
		if c == '}' && f.expect == firstKey {
			p.stack = p.stack[:len(p.stack)-1]
			return
		}
		if c != '"' {
			p.invalid = true
			return
		}
		p.inString = true
		p.readingKey = true
		p.keySize = 0
		f.expect = colon
	case colon:
		p.invalid = c != ':'
		f.expect = objectValue
	case firstElement, nextElement, objectValue:
		if c == ']' && f.expect == firstElement {
			p.stack = p.stack[:len(p.stack)-1]
			return
		}
		if f.expect == objectValue {
			f.expect = objectEnd
		} else {
			f.expect = arrayEnd
		}
		p.startValue(c, f)
	case objectEnd, arrayEnd:
		if c == '}' && f.expect == objectEnd || c == ']' && f.expect == arrayEnd {
			p.stack = p.stack[:len(p.stack)-1]
		} else if c != ',' {
			p.invalid = true
		} else if f.expect == objectEnd {
			f.expect = nextKey
		} else {
			f.expect = nextElement
		}
	}
}

func (p *resultJSON) startValue(c byte, parent *resultFrame) {
	switch c {
	case '{', '[':
		// Match encoding/json's maximum nesting depth.
		if len(p.stack) >= 10000 {
			p.invalid = true
			return
		}
		child := resultFrame{expect: firstKey}
		if c == '[' {
			child.expect = firstElement
			if parent.role == rootObject {
				if p.search && parent.field == "results" {
					p.found = true
				}
				if !p.search && parent.field == "data" {
					child.role = imageArray
				}
			}
		} else if parent.role == imageArray {
			child.role = imageObject
		}
		p.stack = append(p.stack, child)
	case '"':
		p.inString = true
		p.nonempty = false
		p.imageString = parent.role == imageObject && (parent.field == "url" || parent.field == "b64_json")
	case 't':
		p.literal = "rue"
	case 'f':
		p.literal = "alse"
	case 'n':
		p.literal = "ull"
	case '-':
		p.number = numberSign
	case '0':
		p.number = numberZero
	default:
		if c >= '1' && c <= '9' {
			p.number = numberInteger
		} else {
			p.invalid = true
		}
	}
}

func (p *resultJSON) stringByte(c byte) {
	if !p.escape && p.unicodeDigits == 0 && c == '"' {
		if p.readingKey {
			f := &p.stack[len(p.stack)-1]
			f.field = ""
			if (f.role == rootObject || f.role == imageObject) && p.keySize <= len(p.key) {
				// Decode escapes so equivalent spellings of a key match.
				var quoted [len(p.key) + 2]byte
				quoted[0] = '"'
				copy(quoted[1:], p.key[:p.keySize])
				quoted[p.keySize+1] = '"'
				if err := json.Unmarshal(quoted[:p.keySize+2], &f.field); err != nil {
					p.invalid = true
				}
				if f.role == rootObject && f.field == "error" {
					p.hasError = true
				}
			}
		} else if p.imageString && p.nonempty {
			p.found = true
		}
		p.inString, p.readingKey, p.imageString = false, false, false
		return
	}
	if p.readingKey && p.keySize <= len(p.key) {
		if p.keySize < len(p.key) {
			p.key[p.keySize] = c
		}
		p.keySize++
	}
	p.nonempty = true
	switch {
	case p.unicodeDigits > 0:
		p.invalid = !(c >= '0' && c <= '9' || c >= 'A' && c <= 'F' || c >= 'a' && c <= 'f')
		p.unicodeDigits--
	case p.escape:
		p.escape = false
		switch c {
		case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
		case 'u':
			p.unicodeDigits = 4
		default:
			p.invalid = true
		}
	case c == '\\':
		p.escape = true
	case c < 0x20:
		p.invalid = true
	}
}

// Track JSON number grammar without retaining digits or converting their value.
type numberState uint8

const (
	noNumber numberState = iota
	numberSign
	numberZero
	numberInteger
	numberDot
	numberFraction
	numberE
	numberExpSign
	numberExponent
)

func (n numberState) complete() bool {
	return n == numberZero || n == numberInteger || n == numberFraction || n == numberExponent
}
func (n *numberState) consume(c byte) bool {
	switch {
	case c >= '0' && c <= '9':
		switch *n {
		case numberSign:
			if c == '0' {
				*n = numberZero
			} else {
				*n = numberInteger
			}
		case numberInteger, numberFraction, numberExponent:
		case numberDot:
			*n = numberFraction
		case numberE, numberExpSign:
			*n = numberExponent
		default:
			return false
		}
	case c == '.' && (*n == numberZero || *n == numberInteger):
		*n = numberDot
	case (c == 'e' || c == 'E') && (*n == numberZero || *n == numberInteger || *n == numberFraction):
		*n = numberE
	case (c == '+' || c == '-') && *n == numberE:
		*n = numberExpSign
	default:
		return false
	}
	return true
}
