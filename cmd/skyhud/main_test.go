package main

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Julianlg752/skyhud/internal/gpmf/gpmftest"
	"github.com/Julianlg752/skyhud/internal/mp4/mp4test"
)

var created = time.Date(2024, 5, 4, 14, 0, 0, 0, time.UTC)

func original(t *testing.T, dir string) string {
	return mp4test.Build(t, mp4test.Options{Dir: dir, Name: "GX010123.MP4", CreationTime: created,
		Format: "gpmd", Timescale: 1000, SampleDurations: []uint32{1001, 1001},
		Payloads: [][]byte{gpmftest.HERO10Second(3, 40.1), gpmftest.HERO10Second(3, 40.1)}})
}

func stabilized(t *testing.T, dir string, startAfter time.Duration) string {
	return mp4test.Build(t, mp4test.Options{Dir: dir, Name: "GX010123_stabilized.mp4",
		CreationTime: created.Add(startAfter), Format: "hvc1", Timescale: 1000,
		SampleDurations: []uint32{1000}, Payloads: [][]byte{[]byte("frame")}})
}

func call(args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := run(args, &out, &errb)
	return code, out.String(), errb.String()
}

func TestInspectJSON(t *testing.T) {
	code, out, errs := call("inspect", "--json", original(t, t.TempDir()))
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errs)
	}
	var r struct {
		Device   string `json:"device"`
		Channels map[string]struct {
			Points    int     `json:"points"`
			Rate      float64 `json:"rate"`
			Estimated bool    `json:"estimated"`
		} `json:"channels"`
	}
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if r.Device != "HERO10 Black" || r.Channels["speed"].Points != 20 {
		t.Fatalf("inspect = %+v", r)
	}
	if strings.Contains(out, "40.1") {
		t.Fatalf("inspect printed a location: %s", out)
	}
}

func TestInspectStabilizedNamesOriginal(t *testing.T) {
	dir := t.TempDir()
	original(t, dir)
	code, out, errs := call("inspect", "--json", stabilized(t, dir, time.Second))
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errs)
	}
	if !strings.Contains(out, `"original":"GX010123.MP4"`) || !strings.Contains(out, `"trimStart":1`) {
		t.Fatalf("inspect = %s", out)
	}
}

type hudData struct {
	Channels map[string]struct {
		T []float64   `json:"t"`
		V [][]float64 `json:"v"`
	} `json:"channels"`
}

func exportHUD(t *testing.T, args ...string) hudData {
	t.Helper()
	outPath := filepath.Join(t.TempDir(), "clip.hud.json")
	code, _, errs := call(append([]string{"export"}, append(args, "-o", outPath)...)...)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errs)
	}
	b, _ := os.ReadFile(outPath)
	var h hudData
	if err := json.Unmarshal(b, &h); err != nil {
		t.Fatal(err)
	}
	return h
}

func TestExportStabilizedShiftsTime(t *testing.T) {
	dir := t.TempDir()
	original(t, dir)
	h := exportHUD(t, stabilized(t, dir, time.Second))
	sp := h.Channels["speed"]
	if len(sp.T) != 10 || math.Abs(sp.T[0]-0.001) > 0.0005 {
		t.Fatalf("speed = %d points from %v", len(sp.T), sp.T)
	}
}

func TestExportTrimStartOverride(t *testing.T) {
	dir := t.TempDir()
	original(t, dir)
	h := exportHUD(t, stabilized(t, dir, time.Second), "--trim-start", "0")
	if sp := h.Channels["speed"]; sp.T[0] != 0 {
		t.Fatalf("speed starts at %v", sp.T[0])
	}
}

func TestExportMissingOriginal(t *testing.T) {
	outPath := filepath.Join(t.TempDir(), "x.hud.json")
	code, _, errs := call("export", stabilized(t, t.TempDir(), time.Second), "-o", outPath)
	if code != 1 || !strings.Contains(errs, "was not found next to the stabilized clip") {
		t.Fatalf("exit %d: %s", code, errs)
	}
}

func TestExportNoLocationByDefault(t *testing.T) {
	path := original(t, t.TempDir())
	if _, ok := exportHUD(t, path).Channels["gps"]; ok {
		t.Fatal("gps exported without --keep-location")
	}
	if _, ok := exportHUD(t, path, "--keep-location").Channels["gps"]; !ok {
		t.Fatal("gps missing with --keep-location")
	}
}

func TestUsageErrors(t *testing.T) {
	if code, _, _ := call(); code != 1 {
		t.Fatal("no arguments should fail")
	}
	if code, _, errs := call("export", "x.mp4"); code != 1 || !strings.Contains(errs, "-o") {
		t.Fatalf("export without -o: %d %s", code, errs)
	}
}

func TestNoTelemetryGivesGyroflowHint(t *testing.T) {
	path := mp4test.Build(t, mp4test.Options{Name: "flight1.mp4", CreationTime: created, Format: "hvc1",
		Timescale: 1000, SampleDurations: []uint32{1000}, Payloads: [][]byte{[]byte("frame")}})
	code, _, errs := call("inspect", path)
	if code != 1 || !strings.Contains(errs, "--original") || !strings.Contains(errs, "Gyroflow") {
		t.Fatalf("exit %d: %s", code, errs)
	}
}

func TestExportWithOriginalFlag(t *testing.T) {
	orig := original(t, t.TempDir())
	renamed := mp4test.Build(t, mp4test.Options{Name: "flight1.mp4", CreationTime: created.Add(time.Second),
		Format: "hvc1", Timescale: 1000, SampleDurations: []uint32{1000}, Payloads: [][]byte{[]byte("frame")}})
	h := exportHUD(t, renamed, "--original", orig)
	if sp := h.Channels["speed"]; len(sp.T) != 10 {
		t.Fatalf("speed = %d points", len(sp.T))
	}
}

func TestExportRefusesToOverwriteClip(t *testing.T) {
	path := original(t, t.TempDir())
	before, _ := os.Stat(path)
	code, _, errs := call("export", path, "-o", path)
	after, _ := os.Stat(path)
	if code != 1 || !strings.Contains(errs, "would overwrite") || after.Size() != before.Size() {
		t.Fatalf("exit %d: %s", code, errs)
	}
}

func TestExportShiftsGPSLockNotes(t *testing.T) {
	dir := t.TempDir()
	mp4test.Build(t, mp4test.Options{Dir: dir, Name: "GX010123.MP4", CreationTime: created,
		Format: "gpmd", Timescale: 1000, SampleDurations: []uint32{1000, 1000},
		Payloads: [][]byte{gpmftest.HERO10Second(0, 0), gpmftest.HERO10Second(3, 40.1)}})
	outPath := filepath.Join(t.TempDir(), "x.hud.json")
	code, _, errs := call("export", stabilized(t, dir, time.Second), "--trim-start", "0.5", "-o", outPath)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errs)
	}
	b, _ := os.ReadFile(outPath)
	if !strings.Contains(string(b), "no GPS lock from 0.0 s to 0.5 s") {
		t.Fatalf("notes not shifted: %s", b[:min(len(b), 300)])
	}
}

func TestVersion(t *testing.T) {
	if code, out, _ := call("version"); code != 0 || out != "skyhud dev\n" {
		t.Fatalf("%d %q", code, out)
	}
}
