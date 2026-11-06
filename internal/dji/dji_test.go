package dji

import (
	"crypto/rand"
	"errors"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Julianlg752/skyhud/internal/dji/djitest"
	"github.com/Julianlg752/skyhud/internal/gpmf/gpmftest"
	"github.com/Julianlg752/skyhud/internal/mp4/mp4test"
	"github.com/Julianlg752/skyhud/internal/telemetry"
)

var created = time.Date(2025, 6, 1, 8, 30, 18, 0, time.UTC)

const deg = math.Pi / 180

// clip writes a djmd clip with one packet per 1/10 s frame.
func clip(t *testing.T, frames ...djitest.Frame) string {
	t.Helper()
	var payloads [][]byte
	var durs []uint32
	for _, f := range frames {
		payloads = append(payloads, djitest.Packet(f))
		durs = append(durs, 100)
	}
	return mp4test.Build(t, mp4test.Options{Name: "DJI_20250601083018_0001_D.MP4", CreationTime: created,
		Format: "djmd", Timescale: 1000, SampleDurations: durs, Payloads: payloads})
}

func level(n int) [][4]float32 {
	q := make([][4]float32, n)
	for i := range q {
		q[i] = [4]float32{1, 0, 0, 0}
	}
	return q
}

func cruise(n int, north float32) []djitest.Frame {
	var fs []djitest.Frame
	for i := range n {
		fs = append(fs, djitest.Frame{Quats: level(3), Vel: [3]float32{north, 0, 0},
			LatLon: [2]float64{40 * deg, -79 * deg}, HeightMM: float32(10000 + 100*i), Device: "DJI Avata2"})
	}
	return fs
}

func TestReadSpeedHeightOrientation(t *testing.T) {
	fs := cruise(20, 3)
	fs[5].Vel = [3]float32{3, 4, 0}
	tr, err := Read(clip(t, fs...))
	if err != nil {
		t.Fatal(err)
	}
	if tr.Source != "dji" || tr.Device != "DJI Avata2" || !tr.Start.Equal(created) {
		t.Fatalf("track = %s %q %v", tr.Source, tr.Device, tr.Start)
	}
	sp := tr.Channels[telemetry.Speed]
	if len(sp.T) != 20 || sp.V[5][0] != 5 || sp.T[5] != 0.5 {
		t.Fatalf("speed = %v at %v", sp.V[5], sp.T[:6])
	}
	h := tr.Channels[telemetry.Height]
	if math.Abs(h.V[3][0]-10.3) > 1e-4 {
		t.Fatalf("height = %v", h.V[3])
	}
	o := tr.Channels[telemetry.Orientation]
	if len(o.T) != 60 || math.Abs(o.T[1]-1.0/30) > 1e-9 || o.V[0][0] != 1 {
		t.Fatalf("orientation: %d points, t[1] = %v", len(o.T), o.T[1])
	}
	if _, ok := tr.Channels[telemetry.Path]; !ok {
		t.Fatal("no path")
	}
}

func TestGForceEstimated(t *testing.T) {
	tr, err := Read(clip(t, cruise(30, 5)...))
	if err != nil {
		t.Fatal(err)
	}
	g := tr.Channels[telemetry.GForce]
	if !g.Estimated || len(g.V) == 0 {
		t.Fatalf("gforce = %+v", g)
	}
	for _, v := range g.V {
		if math.Abs(v[0]-1) > 0.01 {
			t.Fatalf("constant velocity gives %v g", v[0])
		}
	}
}

func TestImplausibleDropped(t *testing.T) {
	fs := cruise(10, 3)
	fs[2].Quats = [][4]float32{{float32(math.NaN()), 0, 0, 0}}
	fs[4].Vel = [3]float32{900, 0, 0}
	tr, err := Read(clip(t, fs...))
	if err != nil {
		t.Fatal(err)
	}
	if n := len(tr.Channels[telemetry.Speed].T); n != 9 {
		t.Fatalf("%d speed points, want 9", n)
	}
	for _, q := range tr.Channels[telemetry.Orientation].V {
		if math.IsNaN(q[0]) {
			t.Fatal("NaN quaternion kept")
		}
	}
}

func TestMissingFieldNote(t *testing.T) {
	fs := cruise(5, 3)
	for i := range fs {
		fs[i].NoHeight = true
	}
	tr, err := Read(clip(t, fs...))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := tr.Channels[telemetry.Height]; ok {
		t.Fatal("height channel without height data")
	}
	if !strings.Contains(strings.Join(tr.Notes, "|"), "height") {
		t.Fatalf("notes = %q", tr.Notes)
	}
}

func TestNoSerialsStored(t *testing.T) {
	fs := cruise(3, 3)
	fs[0].Serial = "1581F6W8XXXXXXXXSERIAL"
	tr, err := Read(clip(t, fs...))
	if err != nil {
		t.Fatal(err)
	}
	all := tr.Device + strings.Join(tr.Notes, "") + tr.Source
	if strings.Contains(all, "SERIAL") {
		t.Fatal("serial stored")
	}
}

func TestNotDJI(t *testing.T) {
	path := mp4test.Build(t, mp4test.Options{Name: "GX010123.MP4", CreationTime: created, Format: "gpmd",
		Timescale: 1000, SampleDurations: []uint32{1001}, Payloads: [][]byte{gpmftest.HERO10Second(3, 40.1)}})
	if _, err := Read(path); !errors.Is(err, ErrNoTelemetry) {
		t.Fatalf("err = %v", err)
	}
}

func TestGarbagePacket(t *testing.T) {
	junk := make([]byte, 300)
	rand.Read(junk)
	path := mp4test.Build(t, mp4test.Options{Name: "DJI_x.MP4", CreationTime: created, Format: "djmd",
		Timescale: 1000, SampleDurations: []uint32{100, 100}, Payloads: [][]byte{junk, djitest.Packet(cruise(1, 2)[0])}})
	tr, err := Read(path)
	if err != nil {
		return // an error is fine; a panic is not
	}
	if n := len(tr.Channels[telemetry.Speed].T); n != 1 {
		t.Fatalf("%d speed points from one good packet", n)
	}
}

// Real file test: SKYHUD_AVATA=/path/to/DJI_..._D.MP4
func TestRealAvataClip(t *testing.T) {
	path := os.Getenv("SKYHUD_AVATA")
	if path == "" {
		t.Skip("SKYHUD_AVATA not set")
	}
	tr, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	h := tr.Channels[telemetry.Height]
	if tr.Device != "DJI Avata2" || len(h.T) < 7540 || len(h.T) > 7560 {
		t.Fatalf("device %q, %d height points", tr.Device, len(h.T))
	}
	maxSpeed := 0.0
	for _, v := range tr.Channels[telemetry.Speed].V {
		maxSpeed = max(maxSpeed, v[0])
	}
	for _, v := range h.V {
		if v[0] < -5 || v[0] > 200 {
			t.Fatalf("height %v m", v[0])
		}
	}
	if maxSpeed <= 1 || maxSpeed > 60 {
		t.Fatalf("max speed %v m/s", maxSpeed)
	}
	for _, q := range tr.Channels[telemetry.Orientation].V {
		n := math.Sqrt(q[0]*q[0] + q[1]*q[1] + q[2]*q[2] + q[3]*q[3])
		if math.Abs(n-1) > 0.1 {
			t.Fatalf("quaternion norm %v", n)
		}
	}
	// Climbing means negative "down" velocity: compare with the height change.
	if c := climbCorrelation(tr); c < 0.8 {
		t.Fatalf("down velocity vs climb rate correlation %.2f", c)
	}
	t.Logf("%s: %d points, max speed %.1f m/s, notes %q", tr.Device, len(h.T), maxSpeed, tr.Notes)
}

// climbCorrelation correlates -v_down with the height's rate of change,
// both averaged over 1 s windows.
func climbCorrelation(tr *telemetry.Track) float64 {
	h, v := tr.Channels[telemetry.Height], tr.Channels[telemetry.Velocity]
	var a, b []float64
	for i := 60; i+60 < len(h.T) && i < len(v.V); i += 60 {
		a = append(a, (h.V[i+30][0]-h.V[i-30][0])/(h.T[i+30]-h.T[i-30]))
		b = append(b, -v.V[i][2])
	}
	return corr(a, b)
}

func corr(a, b []float64) float64 {
	n := float64(len(a))
	var sa, sb float64
	for i := range a {
		sa += a[i]
		sb += b[i]
	}
	ma, mb := sa/n, sb/n
	var num, da, db float64
	for i := range a {
		num += (a[i] - ma) * (b[i] - mb)
		da += (a[i] - ma) * (a[i] - ma)
		db += (b[i] - mb) * (b[i] - mb)
	}
	return num / math.Sqrt(da*db)
}

func TestZeroHeightLeftOut(t *testing.T) {
	fs := cruise(3, 3)
	fs[0].HeightMM = 0 // protobuf drops zero values: the message is there, the field isn't
	tr, err := Read(clip(t, fs...))
	if err != nil {
		t.Fatal(err)
	}
	h := tr.Channels[telemetry.Height]
	if len(h.T) != 3 || h.V[0][0] != 0 {
		t.Fatalf("height = %v", h.V)
	}
}

// A firmware that moves or retypes fields must read as missing, not zero.
func TestChangedLayoutIsMissing(t *testing.T) {
	fs := cruise(20, 3)
	for i := range fs {
		fs[i].VelField = 4
		fs[i].HeightVarint = true
	}
	tr, err := Read(clip(t, fs...))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []string{telemetry.Speed, telemetry.Height, telemetry.GForce} {
		if s, ok := tr.Channels[c]; ok && len(s.T) > 0 {
			t.Errorf("%s has %d points from a changed layout", c, len(s.T))
		}
	}
	notes := strings.Join(tr.Notes, "|")
	if !strings.Contains(notes, "velocity") || !strings.Contains(notes, "height") {
		t.Fatalf("notes = %q", notes)
	}
}

func TestAllZeroIsMissing(t *testing.T) {
	fs := cruise(20, 0)
	for i := range fs {
		fs[i].HeightMM = 0
	}
	tr, err := Read(clip(t, fs...))
	if err != nil {
		t.Fatal(err)
	}
	if s := tr.Channels[telemetry.Speed]; len(s.T) > 0 {
		t.Errorf("all-zero velocity kept as %d points", len(s.T))
	}
	if !strings.Contains(strings.Join(tr.Notes, "|"), "always zero") {
		t.Fatalf("notes = %q", tr.Notes)
	}
}

func TestOtherDeviceNote(t *testing.T) {
	fs := cruise(5, 3)
	for i := range fs {
		fs[i].Device = "DJI Osmo Action 4"
	}
	tr, err := Read(clip(t, fs...))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(tr.Notes, "|"), "only checked on the DJI Avata 2") {
		t.Fatalf("notes = %q", tr.Notes)
	}
}
