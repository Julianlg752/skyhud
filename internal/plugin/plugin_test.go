package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScriptSetsBinary(t *testing.T) {
	for _, bin := range []string{"/Users/a b/dev/skyhud", `C:\Program Files\SkyHUD\skyhud.exe`} {
		s := string(Script(bin))
		first := s[:strings.Index(s, "\n")]
		if first != "local SKYHUD_BIN = [==["+bin+"]==]" {
			t.Fatalf("first line = %q", first)
		}
		if !strings.Contains(s, "SkyHUD.fill") {
			t.Fatal("script body missing")
		}
	}
}

func TestScriptsDir(t *testing.T) {
	mac := ScriptsDir("darwin", "/Users/j", "")
	if mac != "/Users/j/Library/Application Support/Blackmagic Design/DaVinci Resolve/Fusion/Scripts/Utility" {
		t.Fatalf("darwin = %q", mac)
	}
	win := ScriptsDir("windows", `C:\Users\j`, `C:\Users\j\AppData\Roaming`)
	if win != `C:\Users\j\AppData\Roaming\Blackmagic Design\DaVinci Resolve\Support\Fusion\Scripts\Utility` {
		t.Fatalf("windows = %q", win)
	}
}

func TestInstallWritesFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Scripts", "Utility")
	path, err := Install(dir, "/opt/skyhud")
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(dir, "SkyHUD.lua") {
		t.Fatalf("path = %q", path)
	}
	b, _ := os.ReadFile(path)
	if !strings.HasPrefix(string(b), "local SKYHUD_BIN = [==[/opt/skyhud]==]\n") {
		t.Fatalf("installed script starts %q", string(b[:60]))
	}
}

func TestPathWarning(t *testing.T) {
	if w := PathWarning("windows", `C:\Users\José\skyhud.exe`); !strings.Contains(w, "non-ASCII") {
		t.Fatalf("windows non-ASCII path: %q", w)
	}
	if w := PathWarning("windows", `C:\Tools\skyhud.exe`); w != "" {
		t.Fatalf("ASCII path warned: %q", w)
	}
	if w := PathWarning("darwin", "/Users/José/skyhud"); w != "" {
		t.Fatalf("macOS warned: %q", w)
	}
}

func TestBinPath(t *testing.T) {
	if p := BinPath("darwin", "/Users/j", ""); p != "/Users/j/Library/Application Support/SkyHUD/bin/skyhud" {
		t.Errorf("darwin = %q", p)
	}
	if p := BinPath("windows", `C:\Users\j`, `C:\ProgramData`); p != `C:\ProgramData\SkyHUD\bin\skyhud.exe` {
		t.Errorf("windows = %q", p)
	}
}
