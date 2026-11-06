// Package plugin holds the SkyHUD panel script for DaVinci Resolve and
// installs it into Resolve's Scripts menu.
package plugin

import (
	_ "embed"
	"os"
	"path/filepath"
	"strings"
)

//go:embed SkyHUD.lua
var script []byte

// Script returns the panel script set to run the skyhud binary at binPath.
func Script(binPath string) []byte {
	s := string(script)
	body := s[strings.Index(s, "\n"):]
	return []byte("local SKYHUD_BIN = [==[" + binPath + "]==]" + body)
}

// ScriptsDir is Resolve's folder for scripts shown on every page under
// Workspace > Scripts.
func ScriptsDir(goos, home, appData string) string {
	if goos == "windows" {
		return appData + `\Blackmagic Design\DaVinci Resolve\Support\Fusion\Scripts\Utility`
	}
	return home + "/Library/Application Support/Blackmagic Design/DaVinci Resolve/Fusion/Scripts/Utility"
}

// BinPath is where install keeps its own copy of skyhud, so the panel keeps
// working after the downloaded zip is deleted. On Windows it sits under
// ProgramData, whose path has no user name (Resolve can't pass non-ASCII
// paths there; see PathWarning).
func BinPath(goos, home, programData string) string {
	if goos == "windows" {
		return programData + `\SkyHUD\bin\skyhud.exe`
	}
	return home + "/Library/Application Support/SkyHUD/bin/skyhud"
}

// CopyBinary copies the program at from to to, replacing an older copy.
func CopyBinary(from, to string) error {
	b, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	tmp := to + ".new"
	if err := os.WriteFile(tmp, b, 0o755); err != nil {
		return err
	}
	if err := os.Rename(tmp, to); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// Install writes SkyHUD.lua into dir and returns its path.
func Install(dir, binPath string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "SkyHUD.lua")
	return path, os.WriteFile(path, Script(binPath), 0o644)
}

// PathWarning explains when Resolve's Lua can't run the binary at binPath:
// on Windows it passes commands through the ANSI code page, which garbles
// non-ASCII characters.
func PathWarning(goos, binPath string) string {
	if goos != "windows" {
		return ""
	}
	for _, r := range binPath {
		if r > 127 {
			return "warning: the path to skyhud.exe has non-ASCII characters, which Resolve can't pass on Windows; move skyhud.exe to a folder like C:\\SkyHUD and run install again"
		}
	}
	return ""
}
