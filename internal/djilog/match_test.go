package djilog

import (
	"bytes"
	"encoding/binary"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Julianlg752/skyhud/internal/djilog/djilogtest"
	"github.com/Julianlg752/skyhud/internal/telemetry"
)

// flightHeight is a wavy height profile, so a shifted copy only lines up
// at one offset.
func flightHeight(t float64) float64 { return 10 + 5*math.Sin(t/1.7) + 3*math.Sin(t/0.6) }

// writeLog writes a version 12 log starting at start with n frames at 10 Hz;
// the log's clock runs lag seconds behind the video's.
func writeLog(t *testing.T, dir, aircraft string, start time.Time, n int, lag float64, height func(float64) float64) string {
	t.Helper()
	var f bytes.Buffer
	pre := make([]byte, 100)
	pre[10] = 12
	f.Write(pre)
	details := make([]byte, 436)
	binary.LittleEndian.PutUint64(details[91:], uint64(start.UnixMilli()))
	details[271] = 152
	copy(details[280:312], aircraft)
	f.Write(details)
	e := &encoder{version: 12}
	for i := range n {
		at := float64(i) / 10
		e.record(1, osd(height(at+lag), 1, 0))
		e.record(5, custom(start.Add(time.Duration(at*float64(time.Second)))))
		e.record(4, rc(1024+660, 1024, uint16(364+i%600), 1024-330))
		e.record(7, centerBattery(77, 15.1))
	}
	f.Write(e.buf.Bytes())
	name := "DJIFlightRecord_" + start.In(time.Local).Format("2006-01-02_[15-04-05]") + ".txt"
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, f.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return name
}

// video is a 20 s clip starting at start with the aircraft's height at 30 Hz.
func video(start time.Time, height func(float64) float64) *telemetry.Track {
	tr := &telemetry.Track{Source: "dji", Device: "DJI Avata2", Start: start, Duration: 20}
	for i := 0; i <= 600; i++ {
		at := float64(i) / 30
		tr.Add(telemetry.Height, at, height(at))
	}
	return tr
}

var clipStart = time.Date(2025, 6, 1, 8, 31, 43, 0, time.UTC)

func noKeys(*Log, []byte) ([][]KeyPoint, error) { return nil, nil }

func TestPicksOverlappingLog(t *testing.T) {
	dir := t.TempDir()
	writeLog(t, dir, "DJI Avata 2", clipStart.Add(-30*time.Minute), 300, 0, flightHeight) // an earlier flight
	want := writeLog(t, dir, "DJI Avata 2", clipStart.Add(-60*time.Second), 1000, 0, func(t float64) float64 { return flightHeight(t - 60) })
	r, err := FindAndAlign(dir, video(clipStart, flightHeight), Options{Keychains: noKeys})
	if err != nil {
		t.Fatal(err)
	}
	if r.Log != want || math.Abs(r.Offset) > 0.05 {
		t.Fatalf("log %q offset %v, want %q 0 (notes %q)", r.Log, r.Offset, want, r.Notes)
	}
	s := r.Sticks
	if len(s.T) == 0 || s.V[0][0] < -1 || s.V[0][0] > 1 || s.V[0][1] != -0.5 || s.V[0][2] != 0 || s.V[0][3] != 1 {
		t.Fatalf("sticks = %v", s.V[:1])
	}
	if b := r.Battery; len(b.T) == 0 || b.V[0][0] != 77 || math.Abs(b.V[0][1]-15.1) > 1e-6 {
		t.Fatalf("battery = %v", b.V[:1])
	}
	for _, at := range s.T {
		if at < -1 || at > 21 {
			t.Fatalf("stick time %v outside the clip", at)
		}
	}
}

func TestOtherAircraftIgnored(t *testing.T) {
	dir := t.TempDir()
	writeLog(t, dir, "DJI Mini 4 Pro", clipStart.Add(-60*time.Second), 1000, 0, func(t float64) float64 { return flightHeight(t - 60) })
	r, err := FindAndAlign(dir, video(clipStart, flightHeight), Options{Keychains: noKeys})
	if err != nil || r.Log != "" || !strings.Contains(strings.Join(r.Notes, "|"), "no matching flight log") {
		t.Fatalf("r = %+v, err %v", r, err)
	}
}

func TestLogEndingBeforeClipIgnored(t *testing.T) {
	dir := t.TempDir()
	writeLog(t, dir, "DJI Avata 2", clipStart.Add(-60*time.Second), 300, 0, flightHeight) // ends 30 s before the clip
	if r, _ := FindAndAlign(dir, video(clipStart, flightHeight), Options{Keychains: noKeys}); r.Log != "" {
		t.Fatalf("used %q", r.Log)
	}
}

func TestOffsetFound(t *testing.T) {
	dir := t.TempDir()
	// The log's clock is 0.6 s behind: its frame at log time x holds the
	// height the video shows at x + 0.6.
	writeLog(t, dir, "DJI Avata 2", clipStart.Add(-60*time.Second), 1000, 0.6, func(t float64) float64 { return flightHeight(t - 60) })
	r, err := FindAndAlign(dir, video(clipStart, flightHeight), Options{Keychains: noKeys})
	if err != nil || r.Log == "" {
		t.Fatalf("r = %+v, err %v", r, err)
	}
	// video time = log time + offset
	if math.Abs(r.Offset-0.6) > 0.051 {
		t.Fatalf("offset %v, want 0.6", r.Offset)
	}
}

func TestLowCorrelationRejected(t *testing.T) {
	dir := t.TempDir()
	rng := rand.New(rand.NewSource(1))
	writeLog(t, dir, "DJI Avata 2", clipStart.Add(-60*time.Second), 1000, 0, func(float64) float64 { return rng.Float64() * 20 })
	r, err := FindAndAlign(dir, video(clipStart, flightHeight), Options{Keychains: noKeys})
	if err != nil || r.Log != "" || !strings.Contains(strings.Join(r.Notes, "|"), "does not line up") {
		t.Fatalf("r = %+v, err %v", r, err)
	}
}

func TestNoLogsFolder(t *testing.T) {
	if _, err := FindAndAlign(filepath.Join(t.TempDir(), "missing"), video(clipStart, flightHeight), Options{Keychains: noKeys}); err == nil {
		t.Fatal("missing folder gave no error")
	}
}

// A flight in another time zone: the phone names the log in its local time.
func TestOtherTimeZone(t *testing.T) {
	dir := t.TempDir()
	start := clipStart.Add(-60 * time.Second)
	writeLog(t, dir, "DJI Avata 2", start, 1000, 0, func(t float64) float64 { return flightHeight(t - 60) })
	// rename it as a phone in UTC+9 would
	entries, _ := os.ReadDir(dir)
	tokyo := time.FixedZone("UTC+9", 9*3600)
	name := "DJIFlightRecord_" + start.In(tokyo).Format("2006-01-02_[15-04-05]") + ".txt"
	os.Rename(filepath.Join(dir, entries[0].Name()), filepath.Join(dir, name))
	r, err := FindAndAlign(dir, video(clipStart, flightHeight), Options{Keychains: noKeys})
	if err != nil || r.Log != name {
		t.Fatalf("log %q (want %q), notes %q, err %v", r.Log, name, r.Notes, err)
	}
}

// Encrypted logs that can't cover the clip are never sent to DJI.
func TestKeysOnlyForCoveringLogs(t *testing.T) {
	dir := t.TempDir()
	for _, before := range []time.Duration{2 * time.Hour, 30 * time.Minute, 10 * time.Minute} { // 90 s flights, all over before the clip
		at := clipStart.Add(-before)
		name := "DJIFlightRecord_" + at.In(time.Local).Format("2006-01-02_[15-04-05]") + ".txt"
		os.WriteFile(filepath.Join(dir, name), djilogtest.V14Header("DJI Avata 2", at), 0o644)
	}
	calls := 0
	r, err := FindAndAlign(dir, video(clipStart, flightHeight), Options{Keychains: func(*Log, []byte) ([][]KeyPoint, error) {
		calls++
		return nil, nil
	}})
	if err != nil || calls != 0 || r.Log != "" {
		t.Fatalf("%d key requests, log %q, err %v", calls, r.Log, err)
	}
}

func TestNoStickDataNoSticks(t *testing.T) {
	for _, c := range []struct {
		name string
		rc   func(i int) []byte
	}{
		{"no RC records", nil},
		{"sticks never move (motion controller)", func(int) []byte { return rc(1024, 1024, 1024, 1024) }},
	} {
		dir := t.TempDir()
		start := clipStart.Add(-60 * time.Second)
		var f bytes.Buffer
		pre := make([]byte, 100)
		pre[10] = 12
		f.Write(pre)
		details := make([]byte, 436)
		binary.LittleEndian.PutUint64(details[91:], uint64(start.UnixMilli()))
		copy(details[280:312], "DJI Avata 2")
		f.Write(details)
		e := &encoder{version: 12}
		for i := range 1000 {
			at := float64(i) / 10
			e.record(1, osd(flightHeight(at-60), 1, 0))
			e.record(5, custom(start.Add(time.Duration(at*float64(time.Second)))))
			if c.rc != nil {
				e.record(4, c.rc(i))
			}
			e.record(7, centerBattery(77, 15.1))
		}
		f.Write(e.buf.Bytes())
		os.WriteFile(filepath.Join(dir, "DJIFlightRecord_"+start.In(time.Local).Format("2006-01-02_[15-04-05]")+".txt"), f.Bytes(), 0o644)
		r, err := FindAndAlign(dir, video(clipStart, flightHeight), Options{Keychains: noKeys})
		if err != nil || r.Log == "" {
			t.Fatalf("%s: r = %+v, err %v", c.name, r, err)
		}
		if len(r.Sticks.T) != 0 || len(r.Battery.T) == 0 {
			t.Errorf("%s: %d stick points, %d battery points", c.name, len(r.Sticks.T), len(r.Battery.T))
		}
	}
}
