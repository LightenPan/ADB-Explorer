package appui

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/oldj/switchhosts/internal/store"
)

// findState is the find dialog's state, kept across frames.
type findState struct {
	query       string
	replacement string
	ignoreCase  bool
	regexp      bool
	results     []findResult
	changedAt   string // the query the results are of
	error       string
}

// findResult is one entry with matches, and where its first match is.
type findResult struct {
	ID      string
	Title   string
	Kind    string
	Count   int
	Line    int
	Snippet string
}

// searchEntries looks through the content of every entry.
func (a *App) searchEntries(query string, ignoreCase, useRegexp bool) []findResult {
	if query == "" {
		return nil
	}
	pattern := query
	if !useRegexp {
		pattern = regexp.QuoteMeta(query)
	}
	if ignoreCase {
		pattern = "(?i)" + pattern
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		a.find.error = err.Error()
		return nil
	}
	a.find.error = ""

	var results []findResult
	for _, node := range store.Flatten(a.list) {
		kind := store.NodeType(node)
		if kind != "local" && kind != "remote" {
			continue
		}
		id := store.NodeID(node)
		content, err := store.ReadEntry(a.paths, id)
		if err != nil {
			continue
		}
		matches := re.FindAllStringIndex(content, -1)
		if len(matches) == 0 {
			continue
		}
		first := matches[0]
		line := strings.Count(content[:first[0]], "\n") + 1
		results = append(results, findResult{
			ID:      id,
			Title:   a.NodeTitleText(node),
			Kind:    kind,
			Count:   len(matches),
			Line:    line,
			Snippet: snippetAround(content, first[0], first[1]),
		})
	}
	a.find.changedAt = fmt.Sprintf("%s|%v|%v", query, ignoreCase, useRegexp)
	return results
}

// snippetAround returns the line holding a match, trimmed to a readable
// length.
func snippetAround(content string, start, end int) string {
	lineStart := strings.LastIndexByte(content[:start], '\n') + 1
	lineEnd := len(content)
	if index := strings.IndexByte(content[end:], '\n'); index >= 0 {
		lineEnd = end + index
	}
	line := strings.TrimSpace(content[lineStart:lineEnd])
	runes := []rune(line)
	if len(runes) > 90 {
		line = string(runes[:90]) + "…"
	}
	return line
}

// replaceAll replaces the query in every entry, for the find dialog's
// replace action.
func (a *App) replaceAllWith(query, replacement string, ignoreCase, useRegexp bool) int {
	if query == "" {
		return 0
	}
	pattern := query
	if !useRegexp {
		pattern = regexp.QuoteMeta(query)
	}
	if ignoreCase {
		pattern = "(?i)" + pattern
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		a.find.error = err.Error()
		return 0
	}
	total := 0
	for _, node := range store.Flatten(a.list) {
		kind := store.NodeType(node)
		if kind != "local" {
			continue
		}
		id := store.NodeID(node)
		content, err := store.ReadEntry(a.paths, id)
		if err != nil {
			continue
		}
		if !re.MatchString(content) {
			continue
		}
		// A literal replacement, as the find window does: $1 is text.
		next := re.ReplaceAllLiteralString(content, replacement)
		total += len(re.FindAllStringIndex(content, -1))
		if err := store.WriteEntry(a.paths, id, next); err != nil {
			a.find.error = err.Error()
			return total
		}
		if a.editorID == id {
			a.loadEditor()
			a.discardEditor()
		}
	}
	return total
}
