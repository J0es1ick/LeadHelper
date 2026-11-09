//go:build windows

package storage

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestSecretsProtectedRoundtripAndReplace(t *testing.T) {
	s := Secrets{Path: filepath.Join(t.TempDir(), "vault.bin")}
	for _, value := range []string{"sensitive-fixture-token", "replacement-token"} {
		if e := s.Save([]byte(value)); e != nil {
			t.Fatal(e)
		}
		disk, e := os.ReadFile(s.Path)
		if e != nil || bytes.Contains(disk, []byte(value)) {
			t.Fatal("secret is plaintext", e)
		}
		restored, e := s.Load()
		if e != nil || string(restored) != value {
			t.Fatal("encrypted roundtrip failed", e)
		}
	}
}
