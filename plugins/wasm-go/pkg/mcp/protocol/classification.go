// Copyright (c) 2022 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package protocol

type classificationPath uint8

const (
	classificationOther classificationPath = iota
	classificationRoot
	classificationParams
	classificationMeta
)

type classificationContainer uint8

const (
	classificationObject classificationContainer = iota
	classificationArray
)

type classificationState uint8

const (
	classificationInitial classificationState = iota
	classificationNext
	classificationColon
	classificationCommaOrEnd
)

type classificationKey uint8

const (
	classificationKeyOther classificationKey = iota
	classificationKeyParams
	classificationKeyMeta
	classificationKeyModern
)

const (
	classificationContainerMask byte = 0x01
	classificationStateMask     byte = 0x0e
	classificationPathMask      byte = 0x30
	classificationPendingMask   byte = 0xc0
)

type bodyClassification uint8

const (
	bodyClassificationInvalid bodyClassification = iota
	bodyClassificationLegacy
	bodyClassificationModern
)

// classifyRequestBody is a bounded, non-recursive JSON lexer. It materializes
// neither keys nor values and keeps only one byte of state per open container.
// A complete reserved key directly under root.params._meta is sufficient
// modern identity. Without that identity, only one complete syntactically
// valid JSON value is proven legacy; malformed or trailing input is invalid.
func classifyRequestBody(body []byte) bodyClassification {
	if len(body) > int(LegacyMaxBodyBytes) {
		body = body[:LegacyMaxBodyBytes]
	}
	i := skipClassificationWhitespace(body, 0)
	if i >= len(body) {
		return bodyClassificationInvalid
	}
	stack := make([]byte, 0, 64)
	var ok bool
	i, ok = consumeClassificationValue(body, i, classificationRoot, &stack)
	if !ok {
		return bodyClassificationInvalid
	}

	for len(stack) > 0 {
		i = skipClassificationWhitespace(body, i)
		if i >= len(body) {
			return bodyClassificationInvalid
		}
		top := len(stack) - 1
		frame := stack[top]
		if classificationFrameContainer(frame) == classificationArray {
			switch classificationFrameState(frame) {
			case classificationInitial, classificationNext:
				if body[i] == ']' {
					if classificationFrameState(frame) != classificationInitial {
						return bodyClassificationInvalid
					}
					stack = stack[:top]
					i++
					continue
				}
				stack[top] = setClassificationFrameState(frame, classificationCommaOrEnd)
				var ok bool
				i, ok = consumeClassificationValue(body, i, classificationOther, &stack)
				if !ok {
					return bodyClassificationInvalid
				}
			case classificationCommaOrEnd:
				switch body[i] {
				case ',':
					stack[top] = setClassificationFrameState(frame, classificationNext)
					i++
				case ']':
					stack = stack[:top]
					i++
				default:
					return bodyClassificationInvalid
				}
			default:
				return bodyClassificationInvalid
			}
			continue
		}

		switch classificationFrameState(frame) {
		case classificationInitial, classificationNext:
			if body[i] == '}' {
				if classificationFrameState(frame) != classificationInitial {
					return bodyClassificationInvalid
				}
				stack = stack[:top]
				i++
				continue
			}
			if body[i] != '"' {
				return bodyClassificationInvalid
			}
			var key classificationKey
			var ok bool
			i, key, ok = scanClassificationString(body, i, classificationFramePath(frame))
			if !ok {
				return bodyClassificationInvalid
			}
			if key == classificationKeyModern {
				return bodyClassificationModern
			}
			pending := classificationOther
			switch key {
			case classificationKeyParams:
				pending = classificationParams
			case classificationKeyMeta:
				pending = classificationMeta
			}
			frame = setClassificationFramePending(frame, pending)
			stack[top] = setClassificationFrameState(frame, classificationColon)
		case classificationColon:
			if body[i] != ':' {
				return bodyClassificationInvalid
			}
			i++
			stack[top] = setClassificationFrameState(frame, classificationCommaOrEnd)
			var ok bool
			i, ok = consumeClassificationValue(body, i, classificationFramePending(frame), &stack)
			if !ok {
				return bodyClassificationInvalid
			}
		case classificationCommaOrEnd:
			switch body[i] {
			case ',':
				stack[top] = setClassificationFrameState(frame, classificationNext)
				i++
			case '}':
				stack = stack[:top]
				i++
			default:
				return bodyClassificationInvalid
			}
		default:
			return bodyClassificationInvalid
		}
	}
	if skipClassificationWhitespace(body, i) != len(body) {
		return bodyClassificationInvalid
	}
	return bodyClassificationLegacy
}

func consumeClassificationValue(body []byte, i int, path classificationPath, stack *[]byte) (int, bool) {
	i = skipClassificationWhitespace(body, i)
	if i >= len(body) {
		return i, false
	}
	switch body[i] {
	case '{':
		*stack = append(*stack, makeClassificationFrame(classificationObject, classificationInitial, path, classificationOther))
		return i + 1, true
	case '[':
		*stack = append(*stack, makeClassificationFrame(classificationArray, classificationInitial, classificationOther, classificationOther))
		return i + 1, true
	case '"':
		next, _, ok := scanClassificationString(body, i, classificationOther)
		return next, ok
	case 't':
		return consumeClassificationLiteral(body, i, "true")
	case 'f':
		return consumeClassificationLiteral(body, i, "false")
	case 'n':
		return consumeClassificationLiteral(body, i, "null")
	default:
		return consumeClassificationNumber(body, i)
	}
}

func scanClassificationString(body []byte, start int, path classificationPath) (int, classificationKey, bool) {
	targets := [3]string{}
	keys := [3]classificationKey{}
	count := 0
	switch path {
	case classificationRoot:
		targets[0], keys[0], count = "params", classificationKeyParams, 1
	case classificationParams:
		targets[0], keys[0], count = "_meta", classificationKeyMeta, 1
	case classificationMeta:
		targets[0], keys[0] = MetaProtocolVersion, classificationKeyModern
		targets[1], keys[1] = MetaClientCapabilities, classificationKeyModern
		targets[2], keys[2], count = MetaClientInfo, classificationKeyModern, 3
	}
	matchMask := byte(0)
	if count > 0 {
		matchMask = byte(1<<count) - 1
	}
	decodedLength := 0
	for i := start + 1; i < len(body); {
		value := body[i]
		if value == '"' {
			for targetIndex := 0; targetIndex < count; targetIndex++ {
				if matchMask&(1<<targetIndex) != 0 && decodedLength == len(targets[targetIndex]) {
					return i + 1, keys[targetIndex], true
				}
			}
			return i + 1, classificationKeyOther, true
		}
		var decoded byte
		ascii := true
		switch value {
		case '\\':
			if i+1 >= len(body) {
				return i, classificationKeyOther, false
			}
			escape := body[i+1]
			switch escape {
			case '"', '\\', '/':
				decoded = escape
				i += 2
			case 'b':
				decoded, i = '\b', i+2
			case 'f':
				decoded, i = '\f', i+2
			case 'n':
				decoded, i = '\n', i+2
			case 'r':
				decoded, i = '\r', i+2
			case 't':
				decoded, i = '\t', i+2
			case 'u':
				codePoint, ok := scanClassificationHex(body, i+2)
				if !ok {
					return i, classificationKeyOther, false
				}
				if codePoint <= 0x7f {
					decoded = byte(codePoint)
				} else {
					ascii = false
				}
				i += 6
			default:
				return i, classificationKeyOther, false
			}
		default:
			if value < 0x20 {
				return i, classificationKeyOther, false
			}
			if value < 0x80 {
				decoded = value
			} else {
				ascii = false
			}
			i++
		}
		if !ascii {
			matchMask = 0
		} else {
			for targetIndex := 0; targetIndex < count; targetIndex++ {
				if matchMask&(1<<targetIndex) != 0 &&
					(decodedLength >= len(targets[targetIndex]) || targets[targetIndex][decodedLength] != decoded) {
					matchMask &^= 1 << targetIndex
				}
			}
		}
		decodedLength++
	}
	return len(body), classificationKeyOther, false
}

func scanClassificationHex(body []byte, start int) (uint16, bool) {
	if start+4 > len(body) {
		return 0, false
	}
	var value uint16
	for _, digit := range body[start : start+4] {
		value <<= 4
		switch {
		case digit >= '0' && digit <= '9':
			value |= uint16(digit - '0')
		case digit >= 'a' && digit <= 'f':
			value |= uint16(digit-'a') + 10
		case digit >= 'A' && digit <= 'F':
			value |= uint16(digit-'A') + 10
		default:
			return 0, false
		}
	}
	return value, true
}

func consumeClassificationLiteral(body []byte, start int, literal string) (int, bool) {
	end := start + len(literal)
	if end > len(body) {
		return end, false
	}
	for i := range literal {
		if body[start+i] != literal[i] {
			return end, false
		}
	}
	return end, true
}

func consumeClassificationNumber(body []byte, start int) (int, bool) {
	i := start
	if i < len(body) && body[i] == '-' {
		i++
	}
	if i >= len(body) {
		return i, false
	}
	if body[i] == '0' {
		i++
	} else {
		if body[i] < '1' || body[i] > '9' {
			return i, false
		}
		for i < len(body) && body[i] >= '0' && body[i] <= '9' {
			i++
		}
	}
	if i < len(body) && body[i] == '.' {
		i++
		fractionStart := i
		for i < len(body) && body[i] >= '0' && body[i] <= '9' {
			i++
		}
		if i == fractionStart {
			return i, false
		}
	}
	if i < len(body) && (body[i] == 'e' || body[i] == 'E') {
		i++
		if i < len(body) && (body[i] == '+' || body[i] == '-') {
			i++
		}
		exponentStart := i
		for i < len(body) && body[i] >= '0' && body[i] <= '9' {
			i++
		}
		if i == exponentStart {
			return i, false
		}
	}
	return i, true
}

func skipClassificationWhitespace(body []byte, i int) int {
	for i < len(body) {
		switch body[i] {
		case ' ', '\t', '\n', '\r':
			i++
		default:
			return i
		}
	}
	return i
}

func makeClassificationFrame(container classificationContainer, state classificationState, path, pending classificationPath) byte {
	return byte(container) |
		byte(state)<<1 |
		byte(path)<<4 |
		byte(pending)<<6
}

func classificationFrameContainer(frame byte) classificationContainer {
	return classificationContainer(frame & classificationContainerMask)
}

func classificationFrameState(frame byte) classificationState {
	return classificationState((frame & classificationStateMask) >> 1)
}

func classificationFramePath(frame byte) classificationPath {
	return classificationPath((frame & classificationPathMask) >> 4)
}

func classificationFramePending(frame byte) classificationPath {
	return classificationPath((frame & classificationPendingMask) >> 6)
}

func setClassificationFrameState(frame byte, state classificationState) byte {
	return frame&^classificationStateMask | byte(state)<<1
}

func setClassificationFramePending(frame byte, pending classificationPath) byte {
	return frame&^classificationPendingMask | byte(pending)<<6
}
