// Package hostswrite turns the selected hosts entries into the system hosts
// file: aggregating content, wrapping it in the managed section, writing it
// with elevation when needed, and compensating a failed apply.
package hostswrite

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/oldj/switchhosts/internal/store"
)

// Content markers delimiting the section SwitchHosts owns in append mode.
const (
	StartMarker = "# --- SWITCHHOSTS_CONTENT_START ---"
	EndMarker   = "# --- SWITCHHOSTS_CONTENT_END ---"
)

var writeLock sync.Mutex

// Normalize strips byte order marks that concatenated sources leave behind
// and normalizes line endings. The system hosts file can then always be
// read back and compensated.
func Normalize(content string) string {
	lf := store.NormalizeToLF(content)
	var b strings.Builder
	b.Grow(len(lf))
	for _, line := range splitInclusive(lf) {
		trimmed := strings.TrimLeft(line, " \t\ufeff")
		prefix := line[:len(line)-len(trimmed)]
		for _, r := range prefix {
			if r != '\ufeff' {
				b.WriteRune(r)
			}
		}
		b.WriteString(trimmed)
	}
	return b.String()
}

func splitInclusive(s string) []string {
	var out []string
	for len(s) > 0 {
		i := strings.IndexByte(s, '\n')
		if i < 0 {
			out = append(out, s)
			break
		}
		out = append(out, s[:i+1])
		s = s[i+1:]
	}
	return out
}

// Aggregate joins the content of every switched-on local and remote node.
func Aggregate(nodes []store.NodeJSON, p store.Paths, removeDuplicates bool) (string, error) {
	var chunks []string
	var walk func([]store.NodeJSON) error
	walk = func(nodes []store.NodeJSON) error {
		for _, node := range nodes {
			if store.NodeOn(node) {
				id := store.NodeID(node)
				content, err := store.ReadEntry(p, id)
				if err != nil {
					return err
				}
				chunks = append(chunks, Normalize(content))
			}
			if err := walk(store.Children(node)); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(nodes); err != nil {
		return "", err
	}
	content := strings.Join(chunks, "\n\n")
	if removeDuplicates {
		content = RemoveDuplicates(content)
	}
	return content, nil
}

// RemoveDuplicates drops repeated domains, keeping the first mapping of each
// address family and noting what it dropped.
func RemoveDuplicates(content string) string {
	seen := map[string]bool{}
	var out []string
	eol := "\n"
	if runtime.GOOS == "windows" {
		eol = "\r\n"
	}
	for _, line := range strings.Split(content, "\n") {
		ip, domains, comment := parseLine(line)
		if ip == "" || len(domains) == 0 {
			out = append(out, line)
			continue
		}
		family := "4"
		if strings.Contains(ip, ":") {
			family = "6"
		}
		var kept, dropped []string
		for _, domain := range domains {
			key := domain + "_" + family
			if seen[key] {
				dropped = append(dropped, domain)
				continue
			}
			seen[key] = true
			kept = append(kept, domain)
		}
		if len(kept) > 0 {
			out = append(out, joinLine(ip, kept, comment))
		}
		if len(dropped) > 0 {
			out = append(out, "# invalid hosts (repeated): "+joinLine(ip, dropped, ""))
		}
	}
	return strings.Join(out, eol)
}

func parseLine(line string) (string, []string, string) {
	before, comment, _ := strings.Cut(line, "#")
	fields := strings.Fields(before)
	if len(fields) == 0 {
		return "", nil, strings.TrimSpace(comment)
	}
	return fields[0], fields[1:], strings.TrimSpace(comment)
}

func joinLine(ip string, domains []string, comment string) string {
	line := ip
	if len(domains) > 0 {
		line += " " + strings.Join(domains, " ")
	}
	if comment != "" {
		line += " # " + comment
	}
	return strings.TrimSpace(line)
}

// SystemHostsPath returns the system hosts file's path.
func SystemHostsPath() (string, error) {
	switch runtime.GOOS {
	case "windows":
		root := os.Getenv("SystemRoot")
		if root == "" {
			root = `C:\Windows`
		}
		return filepath.Join(root, "System32", "drivers", "etc", "hosts"), nil
	default:
		return "/etc/hosts", nil
	}
}

// ReadSystemHosts returns the system hosts file's text, with LF endings.
func ReadSystemHosts() (string, []byte, error) {
	path, err := SystemHostsPath()
	if err != nil {
		return "", nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", nil, fmt.Errorf("read %s: %w", path, err)
	}
	return store.NormalizeToLF(string(data)), data, nil
}

// ApplyOutcome describes a successful write.
type ApplyOutcome struct {
	PreviousContent string
	PreviousBytes   []byte
	NewContent      string
	Unchanged       bool
}

// ApplyError is a plain failure of a write.
type ApplyError struct {
	Code    string
	Message string
}

func (e *ApplyError) Error() string { return e.Message }

// Apply writes the aggregated content to the system hosts file.
func Apply(content, writeMode, encodingBackupDir string) (*ApplyOutcome, error) {
	writeLock.Lock()
	defer writeLock.Unlock()

	path, err := SystemHostsPath()
	if err != nil {
		return nil, err
	}
	previousRaw, err := os.ReadFile(path)
	if err != nil {
		return nil, &ApplyError{Code: "no_access", Message: fmt.Sprintf("read %s: %v", path, err)}
	}
	previous := store.NormalizeToLF(string(previousRaw))

	next := Normalize(content)
	if writeMode == "append" {
		next = wrapManagedSection(previous, next)
	}
	if runtime.GOOS == "windows" {
		next = strings.ReplaceAll(next, "\n", "\r\n")
	}
	if next == previous && string(previousRaw) == next {
		return &ApplyOutcome{PreviousContent: previous, PreviousBytes: previousRaw, NewContent: store.NormalizeToLF(next), Unchanged: true}, nil
	}

	// Keep the exact bytes we replaced, so a failed save can restore the
	// encoding a legacy Windows file had.
	if encodingBackupDir != "" && !strings.HasPrefix(string(previousRaw), "\ufeff") {
		_ = os.MkdirAll(encodingBackupDir, 0o755)
	}

	if err := writeFile(path, []byte(next)); err != nil {
		return nil, err
	}
	return &ApplyOutcome{
		PreviousContent: previous,
		PreviousBytes:   previousRaw,
		NewContent:      store.NormalizeToLF(next),
	}, nil
}

// wrapManagedSection rebuilds the hosts file around the markers, keeping
// whatever other tools appended after the managed section.
func wrapManagedSection(previousLF, newContent string) string {
	head, tail := splitManaged(previousLF)
	head = strings.TrimRight(head, "\n \t")
	tail = strings.TrimSpace(tail)
	content := strings.TrimSpace(stripMarkerLines(newContent))

	var parts []string
	if head != "" {
		parts = append(parts, head)
	}
	if content != "" {
		parts = append(parts, StartMarker+"\n\n"+content+"\n\n"+EndMarker)
	} else if tail != "" {
		parts = append(parts, StartMarker+"\n\n"+EndMarker)
	}
	if tail != "" {
		parts = append(parts, tail)
	}
	return strings.Join(parts, "\n\n") + "\n"
}

func splitManaged(previousLF string) (string, string) {
	startBegin, startEnd, ok := findMarker(previousLF, 0, StartMarker)
	if !ok {
		return previousLF, ""
	}
	if _, endEnd, ok := findMarker(previousLF, startEnd, EndMarker); ok {
		return previousLF[:startBegin], previousLF[endEnd:]
	}
	return previousLF[:startBegin], ""
}

func findMarker(text string, from int, marker string) (int, int, bool) {
	pos := from
	for pos <= len(text) {
		end := strings.IndexByte(text[pos:], '\n')
		lineEnd := len(text)
		if end >= 0 {
			lineEnd = pos + end
		}
		if strings.TrimSpace(text[pos:lineEnd]) == marker {
			return pos, lineEnd, true
		}
		if end < 0 {
			break
		}
		pos = lineEnd + 1
	}
	return 0, 0, false
}

func isMarkerLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	return trimmed == StartMarker || trimmed == EndMarker
}

func stripMarkerLines(contentLF string) string {
	if !strings.Contains(contentLF, StartMarker) && !strings.Contains(contentLF, EndMarker) {
		return contentLF
	}
	var kept []string
	for _, line := range strings.Split(contentLF, "\n") {
		if isMarkerLine(line) {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

// Restore writes previous back over the system file, but only when the file
// is still the one this app wrote.
func Restore(previous, expected string) error {
	writeLock.Lock()
	defer writeLock.Unlock()
	path, err := SystemHostsPath()
	if err != nil {
		return err
	}
	current, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if store.NormalizeToLF(string(current)) != store.NormalizeToLF(expected) {
		return &ApplyError{Code: "content_changed", Message: "the hosts file changed since the apply"}
	}
	text := previous
	if runtime.GOOS == "windows" {
		text = strings.ReplaceAll(text, "\n", "\r\n")
	}
	return writeFile(path, []byte(text))
}

// SystemHostsMatches reports whether the file still holds expected.
func SystemHostsMatches(expected string) bool {
	path, err := SystemHostsPath()
	if err != nil {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return store.NormalizeToLF(string(data)) == store.NormalizeToLF(expected)
}

func countRules(content string) int {
	count := 0
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			count++
		}
	}
	return count
}

// CountRules counts the mapping lines of a hosts text.
func CountRules(content string) int { return countRules(content) }
