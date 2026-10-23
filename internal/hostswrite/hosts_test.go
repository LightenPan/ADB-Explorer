package hostswrite

import (
	"testing"
)

// TestAppendModeKeepsTheTailAndMarkers checks the managed section is
// rebuilt around the app's markers, leaving what other tools appended.
func TestAppendModeKeepsTheTailAndMarkers(t *testing.T) {
	cases := []struct {
		name     string
		previous string
		content  string
		want     string
	}{
		{
			name:     "a tail after the markers stays",
			previous: "127.0.0.1 localhost\n" + StartMarker + "\n10.0.0.1 old.test\n" + EndMarker + "\n# another tool\n",
			content:  "10.0.0.2 new.test\n",
			want:     "127.0.0.1 localhost\n\n" + StartMarker + "\n\n10.0.0.2 new.test\n\n" + EndMarker + "\n\n# another tool\n",
		},
		{
			name:     "legacy files without an end marker are rewritten",
			previous: "head\n" + StartMarker + "\nold\n",
			content:  "new\n",
			want:     "head\n\n" + StartMarker + "\n\nnew\n\n" + EndMarker + "\n",
		},
		{
			name:     "no content keeps the anchors and the tail",
			previous: "head\n" + StartMarker + "\nold\n" + EndMarker + "\ntail\n",
			content:  "",
			want:     "head\n\n" + StartMarker + "\n\n" + EndMarker + "\n\ntail\n",
		},
		{
			name:     "marker lines in the content never split the section",
			previous: "head\n",
			content:  "10.0.0.1 a.test\n" + EndMarker + "\n",
			want:     "head\n\n" + StartMarker + "\n\n10.0.0.1 a.test\n\n" + EndMarker + "\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := wrapManagedSection(tc.previous, tc.content)
			if got != tc.want {
				t.Errorf("got:\n%q\nwant:\n%q", got, tc.want)
			}
		})
	}
}

// TestReapplyingIsByteIdentical checks that writing the same selection
// twice produces the same file, so a no-op does not prompt for elevation.
func TestReapplyingIsByteIdentical(t *testing.T) {
	previous := "127.0.0.1 localhost\n"
	first := wrapManagedSection(previous, "10.0.0.1 a.test\n")
	second := wrapManagedSection(first, "10.0.0.1 a.test\n")
	if first != second {
		t.Errorf("second apply differs:\n%q\n%q", first, second)
	}
}

// TestNormalizeStripsMarksAndLineEndings checks the writer's input is
// normalized, as concatenated sources require.
func TestNormalizeStripsMarksAndLineEndings(t *testing.T) {
	got := Normalize("\ufeff127.0.0.1 a.test\r\n\r\n \t\ufeff\ufeff::1 b.test\r# keep \ufeff here\n")
	want := "127.0.0.1 a.test\n\n \t::1 b.test\n# keep \ufeff here\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
