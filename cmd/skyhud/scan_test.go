package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Julianlg752/skyhud/internal/djilog/djilogtest"
	"github.com/Julianlg752/skyhud/internal/mp4/mp4test"
)

func plainVideo(t *testing.T, dir, name string) string {
	return mp4test.Build(t, mp4test.Options{Dir: dir, Name: name, CreationTime: created,
		Format: "hvc1", Timescale: 1000, SampleDurations: []uint32{1000}, Payloads: [][]byte{[]byte("frame")}})
}

func TestScanMixedFolder(t *testing.T) {
	dir := t.TempDir()
	original(t, dir)
	stabilized(t, dir, 0)
	plainVideo(t, dir, "GX010500_stabilized.mp4") // Gyroflow copy, original missing
	plainVideo(t, dir, "IMG_1234.MOV")            // phone clip
	wavyAvata(t, dir)
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("not a video"), 0o644)
	logs := logsFor(t)
	other := time.Date(2024, 5, 4, 9, 0, 0, 0, time.UTC) // same day, another flight
	stray := "DJIFlightRecord_2024-05-04_[09-00-00].txt"
	old := "DJIFlightRecord_2021-02-23_[09-13-52].txt" // years away: not worth listing
	os.WriteFile(filepath.Join(logs, old), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(logs, stray), djilogtest.V12("DJI Avata 2", other,
		[]djilogtest.Frame{{At: other, Height: 1, Throttle: 1024, Rudder: 1024, Elevator: 1024, Aileron: 1024, Battery: 90, Volts: 16}}), 0o644)

	code, out, errs := call("scan", dir, "--logs", logs, "--json")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errs)
	}
	var r scanReport
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	byName := map[string]scanVideo{}
	var names []string
	for _, v := range r.Videos {
		byName[v.File] = v
		names = append(names, v.File)
	}
	want := []string{"DJI_20250601083018_0001_D.MP4", "GX010123.MP4", "GX010123_stabilized.mp4", "GX010500_stabilized.mp4", "IMG_1234.MOV"}
	if !slices.Equal(names, want) {
		t.Fatalf("videos %v, want %v", names, want)
	}
	if v := byName["GX010123.MP4"]; v.Source != "gopro" || !slices.Contains(v.Pieces, "speed") || v.Duration <= 0 || v.Recorded == "" {
		t.Errorf("gopro: %+v", v)
	}
	if v := byName["GX010123_stabilized.mp4"]; v.Original != "GX010123.MP4" || !slices.Contains(v.Pieces, "speed") {
		t.Errorf("stabilized: %+v", v)
	}
	if v := byName["GX010500_stabilized.mp4"]; !strings.Contains(v.Problem, "original") {
		t.Errorf("missing original: %+v", v)
	}
	if v := byName["IMG_1234.MOV"]; !strings.Contains(v.Problem, "no flight data") {
		t.Errorf("phone: %+v", v)
	}
	if v := byName["DJI_20250601083018_0001_D.MP4"]; !strings.HasPrefix(v.Log, "DJIFlightRecord_") || v.Offset == nil ||
		!slices.Contains(v.Pieces, "height") || !slices.Contains(v.Pieces, "sticks") || v.Device == "" {
		t.Errorf("avata: %+v", v)
	}
	for _, g := range []string{"IMG_1234.MOV", "GX010500_stabilized.mp4", stray} {
		if !slices.ContainsFunc(r.Gaps, func(s string) bool { return strings.Contains(s, g) }) {
			t.Errorf("gaps %q lack %s", r.Gaps, g)
		}
	}
	if strings.Contains(out, old) {
		t.Errorf("lists a log from another day: %s", out)
	}
	// Positions never appear (the GoPro sample flies near latitude 40.1).
	if strings.Contains(out, "40.1") || strings.Contains(strings.ToLower(out), "lat") {
		t.Errorf("output shows a position: %s", out)
	}

	code, text, _ := call("scan", dir, "--logs", logs)
	if code != 0 || !strings.Contains(text, "GX010123.MP4") || !strings.Contains(text, "Gaps") || !strings.Contains(text, stray) {
		t.Errorf("text report: %d %q", code, text)
	}
}

func TestScanNoVideos(t *testing.T) {
	code, out, _ := call("scan", t.TempDir())
	if code != 0 || !strings.Contains(out, "No videos") {
		t.Errorf("%d %q", code, out)
	}
}
