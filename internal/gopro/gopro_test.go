package gopro

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Julianlg752/skyhud/internal/gpmf/gpmftest"
	"github.com/Julianlg752/skyhud/internal/mp4/mp4test"
	"github.com/Julianlg752/skyhud/internal/telemetry"
)

var created = time.Date(2024, 5, 4, 14, 0, 0, 0, time.UTC)

// payload builds one second of HERO10-like telemetry.
func payload(fix uint32, lat float64) []byte { return gpmftest.HERO10Second(fix, lat) }

func clip(t *testing.T, payloads ...[]byte) string {
	durs := make([]uint32, len(payloads))
	for i := range durs {
		durs[i] = 1001
	}
	return mp4test.Build(t, mp4test.Options{
		Name: "GX010123.MP4", CreationTime: created, Format: "gpmd",
		Timescale: 1000, SampleDurations: durs, Payloads: payloads,
	})
}

func TestReadSyntheticGoPro(t *testing.T) {
	tr, err := Read(clip(t, payload(3, 40.1), payload(3, 40.1)))
	if err != nil {
		t.Fatal(err)
	}
	sp := tr.Channels[telemetry.Speed]
	if len(sp.T) != 20 || sp.T[0] != 0 || math.Abs(sp.T[10]-1.001) > 0.0005 || math.Abs(sp.V[0][0]-2.15) > 1e-9 {
		t.Fatalf("speed = %d points, T[0]=%v T[10]=%v V[0]=%v", len(sp.T), sp.T[0], sp.T[10], sp.V[0])
	}
	if n := len(tr.Channels[telemetry.Accel].T); n != 400 {
		t.Fatalf("accel points = %d", n)
	}
	if g := tr.Channels[telemetry.GForce].V[0][0]; math.Abs(g-1.0003) > 0.001 {
		t.Fatalf("gforce = %v", g)
	}
	if tr.Device != "HERO10 Black" || tr.Source != "gopro" || !tr.Start.Equal(created) {
		t.Fatalf("track = %+v", tr)
	}
	if !strings.Contains(strings.Join(tr.Notes, "\n"), "ZXY") {
		t.Fatalf("notes missing axis order: %v", tr.Notes)
	}
}

func TestReadNoGPSFix(t *testing.T) {
	tr, err := Read(clip(t, payload(0, 0), payload(3, 40.1)))
	if err != nil {
		t.Fatal(err)
	}
	sp := tr.Channels[telemetry.Speed]
	if len(sp.T) != 10 || sp.T[0] < 1.0 {
		t.Fatalf("speed should start after the lock: %d points, T[0]=%v", len(sp.T), sp.T)
	}
	if !strings.Contains(strings.Join(tr.AllNotes(), "\n"), "no GPS lock from 0.0 s to 1.0 s") {
		t.Fatalf("notes = %v", tr.AllNotes())
	}
}

func TestPathRelativeToFirstFix(t *testing.T) {
	tr, err := Read(clip(t, payload(3, 40.100), payload(3, 40.101)))
	if err != nil {
		t.Fatal(err)
	}
	p := tr.Channels[telemetry.Path]
	last := p.V[len(p.V)-1]
	if math.Abs(last[1]-111.2) > 1 || math.Abs(last[0]) > 0.01 {
		t.Fatalf("last path point = %v", last)
	}
}

func TestReadFileWithoutTelemetry(t *testing.T) {
	path := mp4test.Build(t, mp4test.Options{CreationTime: created, Format: "hvc1", Timescale: 1000,
		SampleDurations: []uint32{1000}, Payloads: [][]byte{[]byte("frame")}})
	if _, err := Read(path); err == nil || !strings.Contains(err.Error(), "no GoPro telemetry") {
		t.Fatalf("err = %v", err)
	}
}

func TestReadRealGoPro(t *testing.T) {
	path := os.Getenv("SKYHUD_GOPRO")
	if path == "" {
		t.Skip("set SKYHUD_GOPRO to a real GoPro clip")
	}
	tr, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if tr.Device != "HERO10 Black" {
		t.Fatalf("Device = %q", tr.Device)
	}
	sp := tr.Channels[telemetry.Speed]
	if n := len(sp.T); n < 3040 || n > 3060 {
		t.Fatalf("speed points = %d", n)
	}
	maxSpeed := 0.0
	for _, v := range sp.V {
		maxSpeed = math.Max(maxSpeed, v[0])
	}
	if maxSpeed < 21 || maxSpeed > 22 {
		t.Fatalf("max speed = %v", maxSpeed)
	}
	if rate := float64(len(tr.Channels[telemetry.Accel].T)) / tr.Duration; rate < 195 || rate > 210 {
		t.Fatalf("accel rate = %v", rate)
	}
	for _, n := range tr.AllNotes() {
		if strings.Contains(n, "GPS lock") {
			t.Fatalf("unexpected note %q", n)
		}
	}
}

func TestReadGPS9PerRowFix(t *testing.T) {
	p := gpmftest.Nest("DEVC", gpmftest.Str("DVNM", "HERO11 Black"), gpmftest.Nest("STRM",
		gpmftest.Int32s("SCAL", []int32{1e7, 1e7, 1000, 1000, 100, 1, 1000, 100, 1}),
		gpmftest.GPS9Rows(
			[9]int64{0, 0, 0, 0, 0, 0, 0, 999, 0},                            // no fix
			[9]int64{401000000, -760123456, 50000, 3000, 3100, 0, 0, 120, 3}, // 3D fix
		),
	))
	tr, err := Read(clip(t, p))
	if err != nil {
		t.Fatal(err)
	}
	sp := tr.Channels[telemetry.Speed]
	if len(sp.T) != 1 || math.Abs(sp.V[0][0]-3) > 1e-9 {
		t.Fatalf("speed = %+v", sp)
	}
}

func TestChapterNote(t *testing.T) {
	path := clip(t, payload(3, 40.1))
	os.WriteFile(filepath.Join(filepath.Dir(path), "GX020123.MP4"), []byte("x"), 0o644)
	tr, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "this clip is chapter 1 of a longer recording; GX020123.MP4 continues it"
	if !strings.Contains(strings.Join(tr.Notes, "\n"), want) {
		t.Fatalf("notes = %v", tr.Notes)
	}
}

func TestLaterChapterNote(t *testing.T) {
	durs := []uint32{1001}
	path := mp4test.Build(t, mp4test.Options{Name: "GX020123.MP4", CreationTime: created, Format: "gpmd",
		Timescale: 1000, SampleDurations: durs, Payloads: [][]byte{payload(3, 40.1)}})
	tr, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "this clip is chapter 2 of a longer recording that starts with GX010123.MP4"
	if !strings.Contains(strings.Join(tr.Notes, "\n"), want) {
		t.Fatalf("notes = %v", tr.Notes)
	}
}

func TestChapterHelpersIgnoreOtherNames(t *testing.T) {
	if got := chapterNote("/tmp/flight.mp4"); got != "" {
		t.Fatalf("note = %q", got)
	}
}
