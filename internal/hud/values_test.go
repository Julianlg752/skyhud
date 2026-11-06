package hud

import (
	"math"
	"testing"

	"github.com/Julianlg752/skyhud/internal/retime"
	"github.com/Julianlg752/skyhud/internal/telemetry"
)

// flight builds 10 s of synthetic data: GPS at 10 Hz with speed = t m/s and
// altitude 775 m with ±5 m alternating noise, g-force at 50 Hz, gravity at
// 30 Hz. gps decides which GPS times are kept.
func flight(gps func(t float64) bool, grav [3]float64) *telemetry.Track {
	tr := &telemetry.Track{Duration: 10}
	for i := 0; i <= 100; i++ {
		t := float64(i) / 10
		if gps == nil || gps(t) {
			noise := 5.0
			if i%2 == 1 {
				noise = -5
			}
			tr.Add(telemetry.Speed, t, t)
			tr.Add(telemetry.Altitude, t, 775+noise)
		}
	}
	for i := 0; i <= 500; i++ {
		tr.Add(telemetry.GForce, float64(i)/50, 1)
	}
	for i := 0; i <= 300; i++ {
		tr.Add(telemetry.Gravity, float64(i)/30, grav[0], grav[1], grav[2])
	}
	return tr
}

var level = [3]float64{0, 1, 0}

func near(t *testing.T, name string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Errorf("%s = %v, want %v ± %v", name, got, want, tol)
	}
}

func TestSpeedFollowsRamp(t *testing.T) {
	v := Sample(flight(nil, level), retime.Map{Start: 2, Speed: 0.5}, 30, 10, 0, SeaLevel)
	s := v[Speed]
	if len(s.V) != 30 || len(s.OK) != 30 {
		t.Fatalf("len = %d/%d", len(s.V), len(s.OK))
	}
	near(t, "frame 0", s.V[0], 7.2, 1e-9)
	near(t, "frame 20", s.V[20], 10.8, 1e-9)
	if !s.OK[0] || !s.OK[20] {
		t.Fatal("speed not OK inside the data")
	}
}

func TestMissingGPSShowsNotOK(t *testing.T) {
	tr := flight(func(t float64) bool { return t >= 1 && (t < 3 || t > 5) }, level)
	tr.NoGPS = [][2]float64{{3, 5}}
	v := Sample(tr, retime.Map{Speed: 1}, 100, 10, 0, SeaLevel)
	for _, p := range []Piece{Speed, Height} {
		if v[p].OK[5] || v[p].OK[40] {
			t.Errorf("%s OK before the first fix or without lock", p)
		}
		if !v[p].OK[70] {
			t.Errorf("%s not OK with lock", p)
		}
	}
	if !v[GForce].OK[5] || !v[GForce].OK[40] {
		t.Error("g-force should not depend on GPS")
	}
}

func TestGapOverOneSecond(t *testing.T) {
	tr := &telemetry.Track{}
	for _, at := range []float64{0, 1.5, 2.5, 3.5} {
		tr.Add(telemetry.Speed, at, 1)
		tr.Add(telemetry.Altitude, at, 100)
	}
	v := Sample(tr, retime.Map{Speed: 1}, 40, 10, 0, SeaLevel)
	if v[Speed].OK[7] {
		t.Error("speed OK across a 1.5 s gap")
	}
	if !v[Speed].OK[20] {
		t.Error("speed not OK across a 1 s gap")
	}
}

func TestHeightModes(t *testing.T) {
	tr := flight(nil, level)
	sea := Sample(tr, retime.Map{Speed: 1}, 100, 10, 0, SeaLevel)[Height]
	start := Sample(tr, retime.Map{Speed: 1}, 100, 10, 0, FromStart)[Height]
	for i := range 100 {
		near(t, "sea level", sea.V[i], 775, 1)
		near(t, "from start", start.V[i], 0, 1)
	}
}

func TestHorizonAngle(t *testing.T) {
	right := Sample(flight(nil, [3]float64{0.5, 0.866, 0}), retime.Map{Speed: 1}, 10, 10, 0, SeaLevel)[Horizon]
	left := Sample(flight(nil, [3]float64{-0.5, 0.866, 0}), retime.Map{Speed: 1}, 10, 10, 0, SeaLevel)[Horizon]
	near(t, "rising right", right.V[3], 30, 0.01)
	near(t, "falling right", left.V[3], -30, 0.01)
	if !right.OK[3] {
		t.Error("horizon not OK")
	}
}

func TestGForceSmoothed(t *testing.T) {
	tr := flight(nil, level)
	s := tr.Channels[telemetry.GForce]
	s.V[250] = []float64{13.7} // one bump at 5 s
	tr.Channels[telemetry.GForce] = s
	g := Sample(tr, retime.Map{Start: 5, Speed: 1}, 1, 10, 0, SeaLevel)[GForce]
	if g.V[0] > 2 {
		t.Errorf("a single 13.7 g bump shows as %.1f g", g.V[0])
	}
}

func TestNudgeBeyondData(t *testing.T) {
	v := Sample(flight(nil, level), retime.Map{Speed: 1}, 20, 10, 100, SeaLevel)
	for _, p := range Pieces {
		for i, ok := range v[p].OK {
			if ok {
				t.Fatalf("%s frame %d OK past the end of the data", p, i)
			}
		}
	}
	empty := Sample(&telemetry.Track{}, retime.Map{Speed: 1}, 5, 10, 0, FromStart)
	for _, p := range Pieces {
		if len(empty[p].V) != 5 || empty[p].OK[0] {
			t.Fatalf("%s on an empty track: %+v", p, empty[p])
		}
	}
	if none := Sample(flight(nil, level), retime.Map{Speed: 1}, 0, 10, 0, SeaLevel); len(none[Speed].V) != 0 {
		t.Fatal("0 frames gave values")
	}
}

// Over water the GPS height wanders around the start height; from the start
// it must never read below zero.
func TestFromStartNeverNegative(t *testing.T) {
	tr := &telemetry.Track{}
	for i := 0; i <= 100; i++ {
		at := float64(i) / 10
		alt := 770.0
		if at > 4 {
			alt = 762 // GPS drifts 8 m below the start, smoothed or not
		}
		tr.Add(telemetry.Speed, at, 1)
		tr.Add(telemetry.Altitude, at, alt)
	}
	h := Sample(tr, retime.Map{Speed: 1}, 100, 10, 0, FromStart)[Height]
	for i, v := range h.V {
		if v < 0 {
			t.Fatalf("frame %d reads %v m", i, v)
		}
	}
	if !h.OK[80] {
		t.Fatal("height not OK")
	}
	sea := Sample(tr, retime.Map{Speed: 1}, 100, 10, 0, SeaLevel)[Height]
	near(t, "sea level keeps the drift", sea.V[80], 762, 0.01)
}

func avataTrack() *telemetry.Track {
	tr := &telemetry.Track{}
	for i := 0; i <= 100; i++ {
		at := float64(i) / 10
		tr.Add(telemetry.Speed, at, 5)
		tr.Add(telemetry.Height, at, 12)
		tr.Add(telemetry.GForce, at, 1)
		// a 30 deg roll about the forward (x) axis: w = cos 15, x = sin 15
		tr.Add(telemetry.Orientation, at, math.Cos(15*math.Pi/180), math.Sin(15*math.Pi/180), 0, 0)
	}
	return tr
}

func TestAvataHeightUsed(t *testing.T) {
	for _, mode := range []HeightMode{SeaLevel, FromStart} {
		h := Sample(avataTrack(), retime.Map{Speed: 1}, 10, 10, 0, mode)[Height]
		if !h.OK[5] {
			t.Fatalf("%s: height not OK", mode)
		}
		// The aircraft's height is already above takeoff, and the video may
		// start in the air: both modes show it as is.
		near(t, string(mode), h.V[5], 12, 1e-9)
	}
}

func TestHorizonFromOrientation(t *testing.T) {
	h := Sample(avataTrack(), retime.Map{Speed: 1}, 10, 10, 0, FromStart)[Horizon]
	if !h.OK[3] {
		t.Fatal("horizon not OK")
	}
	near(t, "roll", math.Abs(h.V[3]), 30, 0.01)
}

func TestSticksAndBatteryValues(t *testing.T) {
	tr := avataTrack()
	for i := 0; i <= 20; i++ {
		at := float64(i) / 2
		tr.Add(telemetry.Sticks, at, 1, -0.5, 0.25, 0)
		tr.Add(telemetry.Battery, at, 78, 15.2)
	}
	v := Sample(tr, retime.Map{Speed: 1}, 10, 10, 0, FromStart)
	if !v[Sticks].OK[4] || !v[Battery].OK[4] {
		t.Fatal("sticks or battery not OK")
	}
	near(t, "throttle", v[StickThrottle].V[4], 1, 1e-9)
	near(t, "yaw", v[StickYaw].V[4], -0.5, 1e-9)
	near(t, "pitch", v[StickPitch].V[4], 0.25, 1e-9)
	near(t, "battery", v[Battery].V[4], 78, 1e-9)
	near(t, "volts", v[BatteryVolts].V[4], 15.2, 1e-9)
	if none := Sample(flight(nil, level), retime.Map{Speed: 1}, 5, 10, 0, FromStart); none[Sticks].OK[2] {
		t.Fatal("sticks OK without stick data")
	}
}

// squarePath flies a 100 m square at 10 m/s with GPS at 10 Hz.
func squarePath() *telemetry.Track {
	tr := &telemetry.Track{}
	corners := [][2]float64{{0, 0}, {100, 0}, {100, 100}, {0, 100}, {0, 0}}
	at := 0.0
	for c := 0; c+1 < len(corners); c++ {
		a, b := corners[c], corners[c+1]
		for i := range 100 {
			f := float64(i) / 100
			tr.Add(telemetry.Path, at, a[0]+(b[0]-a[0])*f, a[1]+(b[1]-a[1])*f)
			tr.Add(telemetry.Speed, at, 10)
			at += 0.1
		}
	}
	tr.Add(telemetry.Path, at, 0, 0)
	return tr
}

func TestPathBoxAndProgress(t *testing.T) {
	tr := squarePath()
	pts := PathPoints(tr, 50)
	if len(pts) == 0 || len(pts) > 50 {
		t.Fatalf("%d points", len(pts))
	}
	for _, p := range pts {
		if math.Abs(p[0]) > 0.5+1e-9 || math.Abs(p[1]) > 0.5+1e-9 {
			t.Fatalf("point %v outside the box", p)
		}
	}
	v := Sample(tr, retime.Map{Speed: 1}, 400, 10, 0, FromStart)
	near(t, "progress halfway", v[PathProgress].V[200], 0.5, 0.01)
	near(t, "x at the second corner", v[PathX].V[100], 0.5, 0.01)
	near(t, "y at the start", v[PathY].V[0], -0.5, 0.01)
	if !v[Path].OK[200] {
		t.Fatal("path not OK")
	}
}

func TestNoPathWithoutGPS(t *testing.T) {
	v := Sample(flight(nil, level), retime.Map{Speed: 1}, 20, 10, 0, FromStart)
	for i, ok := range v[Path].OK {
		if ok {
			t.Fatalf("path OK at frame %d without path data", i)
		}
	}
	if len(PathPoints(flight(nil, level), 50)) != 0 {
		t.Fatal("path points without path data")
	}
}

// rollQuat is a rotation about the forward axis.
func rollQuat(deg float64) []float64 {
	h := deg * math.Pi / 360
	return []float64{math.Cos(h), math.Sin(h), 0, 0}
}

func TestFlipsFromOrientation(t *testing.T) {
	tr := &telemetry.Track{}
	for i := 0; i <= 100; i++ {
		at := float64(i) / 10
		roll := 0.0
		if at >= 4 && at < 5 { // a roll flip from 4 s to 5 s
			roll = (at - 4) * 360
		}
		tr.Add(telemetry.Orientation, at, rollQuat(roll)...)
	}
	flips := Flips(tr)
	if len(flips) != 1 || math.Abs(flips[0]-4.3) > 0.11 {
		t.Fatalf("flips = %v", flips)
	}
	v := Sample(tr, retime.Map{Speed: 1}, 100, 10, 0, FromStart)
	if v[Flip].V[45] != 1 || v[Flip].V[20] != 0 || !v[Flip].OK[20] {
		t.Fatalf("inverted at 4.5 s = %v, at 2 s = %v", v[Flip].V[45], v[Flip].V[20])
	}
	near(t, "flip age at 6 s", v[FlipAge].V[60], 6-4.3, 0.11)
	near(t, "flip age before", v[FlipAge].V[10], 99, 1e-9)
}

func TestFlipsFromGravity(t *testing.T) {
	tr := &telemetry.Track{}
	for i := 0; i <= 50; i++ {
		at := float64(i) / 10
		y := 1.0
		if at >= 2 && at < 3 {
			y = -1
		}
		tr.Add(telemetry.Gravity, at, 0, y, 0)
	}
	if flips := Flips(tr); len(flips) != 1 || math.Abs(flips[0]-2) > 0.11 {
		t.Fatalf("flips = %v", flips)
	}
}

func TestPitchSign(t *testing.T) {
	tr := &telemetry.Track{}
	h := -20 * math.Pi / 360 // nose 20 deg down: rotation about the right axis
	for i := 0; i <= 10; i++ {
		tr.Add(telemetry.Orientation, float64(i)/10, math.Cos(h), 0, math.Sin(h), 0)
	}
	v := Sample(tr, retime.Map{Speed: 1}, 5, 10, 0, FromStart)
	near(t, "pitch", v[Pitch].V[2], -20, 0.01)
}
