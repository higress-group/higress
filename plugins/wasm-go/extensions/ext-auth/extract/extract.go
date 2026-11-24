package extract

import (
	"net/http"
	"strconv"

	"github.com/tidwall/gjson"
)

// Source enumerates where a value is extracted from within the authorization response.
const (
	SourceStatusCode = "status_code"
	SourceHeader     = "header"
	SourceBodyJson   = "body_json"
)

// Value extracts one string value from the authorization response by source and key.
// ok is false when nothing can be extracted: an unknown source, a missing header,
// a non-existent gjson path, or an empty body.
func Value(source, key string, statusCode int, headers http.Header, body []byte) (string, bool) {
	switch source {
	case SourceStatusCode:
		return strconv.Itoa(statusCode), true
	case SourceHeader:
		if v := headers.Get(key); v != "" {
			return v, true
		}
		return "", false
	case SourceBodyJson:
		if len(body) == 0 {
			return "", false
		}
		if g := gjson.GetBytes(body, key); g.Exists() {
			return g.String(), true
		}
		return "", false
	default:
		return "", false
	}
}
