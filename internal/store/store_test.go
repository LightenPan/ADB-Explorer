package store

import (
	"os"
	"testing"
)

func tempPaths(t *testing.T) Paths {
	t.Helper()
	p := Under(t.TempDir())
	if err := p.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestEntryIDCannotEscapeTheDirectory checks unsafe ids are refused.
func TestEntryIDCannotEscapeTheDirectory(t *testing.T) {
	p := tempPaths(t)
	for _, id := range []string{"", "../escape", `..\escape`, "a/b"} {
		if _, err := entryPath(p, id); err == nil {
			t.Errorf("id %q was accepted", id)
		}
	}
}

// TestTransactionRollsBack checks a failed action restores every target.
func TestTransactionRollsBack(t *testing.T) {
	p := tempPaths(t)
	original := []byte("original\n")
	if err := AtomicWrite(p.ManifestFile, original); err != nil {
		t.Fatal(err)
	}
	err := Tx(p, []TxTarget{{Kind: "manifest"}, {Kind: "trashcan"}}, func() error {
		if err := AtomicWrite(p.ManifestFile, []byte("changed\n")); err != nil {
			return err
		}
		return os.ErrPermission
	})
	if err == nil {
		t.Fatal("the transaction reported success")
	}
	data, readErr := os.ReadFile(p.ManifestFile)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != string(original) {
		t.Errorf("the manifest is %q after a rollback", data)
	}
	if _, statErr := os.Stat(p.TrashcanFile); !os.IsNotExist(statErr) {
		t.Error("the trashcan was left behind by a failed transaction")
	}
}
