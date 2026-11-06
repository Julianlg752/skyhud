package secret

import (
	"errors"
	"os"
	"runtime"
	"testing"
)

func TestFakeStore(t *testing.T) {
	f := NewMemory()
	if _, err := f.Get("x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
	if err := f.Set("x", "v"); err != nil {
		t.Fatal(err)
	}
	if v, _ := f.Get("x"); v != "v" {
		t.Fatalf("got %q", v)
	}
}

// Writes to the real keychain under a test name; opt in with
// SKYHUD_KEYCHAIN_TEST=1.
func TestKeychainRoundTrip(t *testing.T) {
	if os.Getenv("SKYHUD_KEYCHAIN_TEST") != "1" || runtime.GOOS != "darwin" {
		t.Skip("set SKYHUD_KEYCHAIN_TEST=1 on macOS")
	}
	const name = "skyhud-test-entry"
	t.Cleanup(func() { System.Delete(name) })
	if err := System.Set(name, `a"b c'd`); err != nil {
		t.Fatal(err)
	}
	if v, err := System.Get(name); err != nil || v != `a"b c'd` {
		t.Fatalf("got %q, %v", v, err)
	}
	System.Delete(name)
	if _, err := System.Get(name); !errors.Is(err, ErrNotFound) {
		t.Fatalf("after delete: %v", err)
	}
}
