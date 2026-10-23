package store

import (
	"encoding/json"
	"strings"
	"time"
)

// NodeJSON is a node of the hosts tree as the rest of the app sees it: the
// flat object with snake_case fields.
type NodeJSON = map[string]any

func unmarshalJSON(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

func marshalIndentJSON(v any) ([]byte, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// NowMillis is the wall clock in milliseconds.
func NowMillis() int64 {
	return time.Now().UnixMilli()
}

// FormatTimestamp renders a time the way the app's lists show it.
func FormatTimestamp(ms int64) string {
	return time.UnixMilli(ms).Format("2006-01-02 15:04:05")
}

// NormalizeToLF collapses CRLF and lone CR to LF.
func NormalizeToLF(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\r", "\n")
}
