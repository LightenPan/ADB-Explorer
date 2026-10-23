package appui

import "strings"

func stringsTrimLeftSpace(s string) string { return strings.TrimLeft(s, " \t") }

func stringsHasPrefix(s, prefix string) bool { return strings.HasPrefix(s, prefix) }

// splitLines splits text into its display lines, without the line ending.
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// splitAddress splits a hosts line into its address and the rest, "" when
// the line holds no address.
func splitAddress(line string) (string, string) {
	trimmed := strings.TrimLeft(line, " \t")
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return "", line
	}
	end := strings.IndexAny(trimmed, " \t")
	if end < 0 {
		return "", line
	}
	address := trimmed[:end]
	if !looksLikeAddress(address) {
		return "", line
	}
	offset := strings.Index(line, address)
	return address, line[offset+len(address):]
}

// looksLikeAddress reports whether a word is an IPv4 or IPv6 address.
func looksLikeAddress(word string) bool {
	if word == "" {
		return false
	}
	if strings.Contains(word, ":") {
		for _, r := range word {
			if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F' || r == ':' || r == '.' || r == '%') {
				return false
			}
		}
		return true
	}
	parts := strings.Split(word, ".")
	if len(parts) != 4 {
		return false
	}
	for _, part := range parts {
		if part == "" || len(part) > 3 {
			return false
		}
		for _, r := range part {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}
