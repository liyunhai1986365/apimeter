package media

import (
	"encoding/base64"
	"strings"
	"unicode/utf8"
)

func IsTextMimeType(mimeType string) bool {
	value, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(mimeType)), ";")
	return strings.HasPrefix(strings.TrimSpace(value), "text/") || strings.TrimSpace(value) == "application/json"
}
func DecodeText(data string) (string, bool) {
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(data))
	if err != nil || !utf8.Valid(decoded) {
		return "", false
	}
	return string(decoded), true
}
