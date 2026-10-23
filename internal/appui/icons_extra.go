package appui

import (
	"github.com/oldj/switchhosts/internal/store"
)

// prettyBytes renders a size the way the status bar shows it.
func prettyBytes(n int) string {
	const unit = 1000
	if n < unit {
		return itoa(int64(n)) + " B"
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	units := []string{"kB", "MB", "GB", "TB", "PB", "EB"}
	if exp >= len(units) {
		exp = len(units) - 1
	}
	return trimFloat(float64(n)/float64(div)) + " " + units[exp]
}

// domainList returns a domain-sourced remote entry's domains.
func domainList(node store.NodeJSON) []string {
	var out []string
	if domains, ok := node["domains"].([]any); ok {
		for _, d := range domains {
			if s, ok := d.(string); ok {
				out = append(out, s)
			}
		}
	}
	if len(out) == 0 {
		if url, _ := node["url"].(string); url != "" {
			out = append(out, url)
		}
	}
	return out
}
