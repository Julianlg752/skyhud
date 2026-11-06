// Package hud turns telemetry into per-frame HUD values for one shot and
// writes them as a Fusion composition that Resolve loads onto the timeline.
package hud

import (
	"math"
	"sort"

	"github.com/Julianlg752/skyhud/internal/retime"
	"github.com/Julianlg752/skyhud/internal/telemetry"
)

// Piece is one element of the HUD.
type Piece string

const (
	Speed   Piece = "speed"   // km/h
	Height  Piece = "height"  // m
	GForce  Piece = "gforce"  // g
	Horizon Piece = "horizon" // degrees, counterclockwise positive
	Sticks  Piece = "sticks"  // OK flags only; the values are the Stick* series
	Battery Piece = "battery" // percent
	Path    Piece = "path"    // OK flags only; see PathX, PathY, PathProgress
	Flip    Piece = "flip"    // 1 while upside down
)

// Data-only series that belong to a piece.
const (
	StickThrottle Piece = "sticks.throttle" // -1..1
	StickYaw      Piece = "sticks.yaw"
	StickPitch    Piece = "sticks.pitch"
	StickRoll     Piece = "sticks.roll"
	BatteryVolts  Piece = "battery.volts"
	Pitch         Piece = "pitch"  // degrees, nose up positive
	PathX         Piece = "path.x" // -0.5..0.5 in the path box (see PathPoints)
	PathY         Piece = "path.y"
	PathProgress  Piece = "path.progress" // 0..1 along the flight by distance
	FlipAge       Piece = "flip.age"      // seconds since the last flip started, 99 if none
)

// Pieces lists every piece in drawing order.
var Pieces = []Piece{Speed, Height, GForce, Horizon, Sticks, Battery, Path, Flip}

var dataSeries = []Piece{StickThrottle, StickYaw, StickPitch, StickRoll, BatteryVolts, Pitch, PathX, PathY, PathProgress, FlipAge}

// HeightMode chooses what the height piece measures from.
type HeightMode string

const (
	SeaLevel  HeightMode = "sea"   // GPS altitude above sea level
	FromStart HeightMode = "start" // height above the start of the recording (takeoff), never below 0
)

// Series holds one value per output frame and whether it is known there.
type Series struct {
	V  []float64
	OK []bool
}

// Values holds every piece's series for one shot.
type Values map[Piece]Series

const (
	maxGap       = 1.0 // s; data points further apart than this leave a hole
	heightWindow = 2.0 // s; centered mean that calms GPS altitude noise
	gforceWindow = 0.5 // s; centered mean that drops single bumps
	droneHeight  = 0.3 // s; light smoothing for the aircraft's own height
	kmhPerMS     = 3.6 // m/s -> km/h
)

// Sample computes each piece for frames output frames at fps. Frame i shows
// the data at m.SourceAt(i/fps) + nudge seconds.
func Sample(tr *telemetry.Track, m retime.Map, frames int, fps, nudge float64, mode HeightMode) Values {
	speed := column(tr, telemetry.Speed, 0)
	// The aircraft's own height (Avata 2) is already above takeoff; GPS
	// altitude (GoPro) needs heavy smoothing.
	height, gpsHeight := column(tr, telemetry.Height, 0).smooth(droneHeight), false
	if len(height.v) == 0 {
		height, gpsHeight = column(tr, telemetry.Altitude, 0).smooth(heightWindow), true
	}
	if (mode == FromStart || !gpsHeight) && len(height.v) > 0 {
		// Height above takeoff never reads below 0: GPS height wanders by
		// meters, and dips below takeoff are almost always noise. The
		// aircraft's own height is already above takeoff (the video may start
		// in the air), so only GPS height is measured from the first value.
		first := 0.0
		if gpsHeight {
			first = height.v[0]
		}
		for i := range height.v {
			height.v[i] = max(height.v[i]-first, 0)
		}
	}
	g := column(tr, telemetry.GForce, 0).smooth(gforceWindow)
	gx, gy := column(tr, telemetry.Gravity, 0), column(tr, telemetry.Gravity, 1)
	roll := rollColumn(tr)
	var sticks [4]points
	for k := range sticks {
		sticks[k] = column(tr, telemetry.Sticks, k)
	}
	pct, volts := column(tr, telemetry.Battery, 0), column(tr, telemetry.Battery, 1)
	pitch, up := attitude(tr)
	px, py, prog := pathColumns(tr)
	flips := Flips(tr)

	out := Values{}
	for _, p := range append(append([]Piece{}, Pieces...), dataSeries...) {
		out[p] = Series{V: make([]float64, max(frames, 0)), OK: make([]bool, max(frames, 0))}
	}
	stickPieces := [4]Piece{StickThrottle, StickYaw, StickPitch, StickRoll}
	for i := 0; i < frames; i++ {
		t := m.SourceAt(float64(i)/fps) + nudge
		lock := !inSpans(tr.NoGPS, t)
		if v, ok := speed.at(t); ok && lock {
			out[Speed].V[i], out[Speed].OK[i] = v*kmhPerMS, true
		}
		if v, ok := height.at(t); ok && (lock || !gpsHeight) {
			out[Height].V[i], out[Height].OK[i] = v, true
		}
		if v, ok := g.at(t); ok {
			out[GForce].V[i], out[GForce].OK[i] = v, true
		}
		x, okx := gx.at(t)
		y, oky := gy.at(t)
		if okx && oky {
			out[Horizon].V[i], out[Horizon].OK[i] = math.Atan2(x, y)*180/math.Pi, true
		} else if v, ok := roll.at(t); ok {
			out[Horizon].V[i], out[Horizon].OK[i] = v, true
		}
		all := true
		for k, p := range stickPieces {
			v, ok := sticks[k].at(t)
			out[p].V[i], out[p].OK[i] = v, ok
			all = all && ok
		}
		out[Sticks].OK[i] = all
		p, okp := pct.at(t)
		v, okv := volts.at(t)
		out[Battery].V[i], out[BatteryVolts].V[i] = p, v
		out[Battery].OK[i] = okp && okv
		out[BatteryVolts].OK[i] = okp && okv
		if v, ok := pitch.at(t); ok {
			out[Pitch].V[i], out[Pitch].OK[i] = v, true
		}
		if v, ok := up.at(t); ok {
			out[Flip].OK[i] = true
			if v < 0 {
				out[Flip].V[i] = 1
			}
		}
		age := flipAgeCap
		for _, f := range flips {
			if f <= t {
				age = min(t-f, flipAgeCap)
			}
		}
		out[FlipAge].V[i], out[FlipAge].OK[i] = age, true
		pxv, okx := px.at(t)
		pyv, oky := py.at(t)
		pg, okg := prog.at(t)
		if okx && oky && okg && lock {
			out[PathX].V[i], out[PathY].V[i], out[PathProgress].V[i] = pxv, pyv, pg
			out[Path].OK[i], out[PathX].OK[i], out[PathY].OK[i], out[PathProgress].OK[i] = true, true, true, true
		}
	}
	return out
}

// rollColumn turns orientation quaternions (w, x, y, z; x forward) into the
// horizon angle in degrees: the roll about the forward axis.
func rollColumn(tr *telemetry.Track) points {
	s := tr.Channels[telemetry.Orientation]
	var p points
	for i, q := range s.V {
		if len(q) < 4 || i >= len(s.T) {
			continue
		}
		w, x, y, z := q[0], q[1], q[2], q[3]
		p.t = append(p.t, s.T[i])
		p.v = append(p.v, math.Atan2(2*(w*x+y*z), 1-2*(x*x+y*y))*180/math.Pi)
	}
	return p
}

// points is one column of a channel.
type points struct{ t, v []float64 }

func column(tr *telemetry.Track, name string, col int) points {
	s := tr.Channels[name]
	var p points
	for i, row := range s.V {
		if col < len(row) && i < len(s.T) {
			p.t = append(p.t, s.T[i])
			p.v = append(p.v, row[col])
		}
	}
	return p
}

// smooth returns the centered moving mean over window seconds.
func (p points) smooth(window float64) points {
	out := points{t: p.t, v: make([]float64, len(p.v))}
	lo, hi, sum := 0, 0, 0.0
	for i, t := range p.t {
		for hi < len(p.t) && p.t[hi] <= t+window/2 {
			sum += p.v[hi]
			hi++
		}
		for p.t[lo] < t-window/2 {
			sum -= p.v[lo]
			lo++
		}
		out.v[i] = sum / float64(hi-lo)
	}
	return out
}

// at interpolates linearly at t. It fails outside the data and between
// points more than maxGap apart.
func (p points) at(t float64) (float64, bool) {
	n := len(p.t)
	if n == 0 || t < p.t[0] || t > p.t[n-1] {
		return 0, false
	}
	j := sort.SearchFloat64s(p.t, t) // first index with p.t[j] >= t
	if p.t[j] == t {
		return p.v[j], true
	}
	a, b := j-1, j
	if p.t[b]-p.t[a] > maxGap {
		return 0, false
	}
	f := (t - p.t[a]) / (p.t[b] - p.t[a])
	return p.v[a] + f*(p.v[b]-p.v[a]), true
}

func inSpans(spans [][2]float64, t float64) bool {
	for _, s := range spans {
		if t >= s[0] && t <= s[1] {
			return true
		}
	}
	return false
}

const (
	flipAgeCap  = 99.0
	minFlipTime = 0.2 // s upside down to count as a flip
)

// attitude returns pitch (degrees, nose up positive) and an "up" value that
// is negative while upside down: from the gravity vector when the track has
// one (GoPro), else from orientation quaternions (w, x, y, z; x forward,
// y right, z down, as on the Avata 2).
func attitude(tr *telemetry.Track) (pitch, up points) {
	if g := tr.Channels[telemetry.Gravity]; len(g.T) > 0 {
		for i, v := range g.V {
			if len(v) < 3 || i >= len(g.T) {
				continue
			}
			pitch.t, up.t = append(pitch.t, g.T[i]), append(up.t, g.T[i])
			pitch.v = append(pitch.v, -math.Atan2(v[2], math.Hypot(v[0], v[1]))*180/math.Pi)
			up.v = append(up.v, v[1])
		}
		return
	}
	o := tr.Channels[telemetry.Orientation]
	for i, q := range o.V {
		if len(q) < 4 || i >= len(o.T) {
			continue
		}
		w, x, y, z := q[0], q[1], q[2], q[3]
		pitch.t, up.t = append(pitch.t, o.T[i]), append(up.t, o.T[i])
		pitch.v = append(pitch.v, math.Asin(max(-1, min(1, 2*(w*y-z*x))))*180/math.Pi)
		up.v = append(up.v, 1-2*(x*x+y*y)) // the body's down axis, seen from the world's down axis
	}
	return
}

// Flips returns the source times when the aircraft turned upside down for
// at least minFlipTime.
func Flips(tr *telemetry.Track) []float64 {
	_, up := attitude(tr)
	var out []float64
	start := -1.0
	for i, t := range up.t {
		switch {
		case up.v[i] < 0 && start < 0:
			start = t
		case up.v[i] >= 0 && start >= 0:
			if t-start >= minFlipTime {
				out = append(out, start)
			}
			start = -1
		}
	}
	if start >= 0 && len(up.t) > 0 && up.t[len(up.t)-1]-start >= minFlipTime {
		out = append(out, start)
	}
	return out
}

// pathBox maps path meters into the box -0.5..0.5, keeping the shape.
func pathBox(tr *telemetry.Track) (xs, ys, ts []float64, scale, cx, cy float64) {
	p := tr.Channels[telemetry.Path]
	minX, maxX, minY, maxY := math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)
	for i, v := range p.V {
		if len(v) < 2 || i >= len(p.T) {
			continue
		}
		xs, ys, ts = append(xs, v[0]), append(ys, v[1]), append(ts, p.T[i])
		minX, maxX, minY, maxY = min(minX, v[0]), max(maxX, v[0]), min(minY, v[1]), max(maxY, v[1])
	}
	if len(xs) == 0 {
		return nil, nil, nil, 0, 0, 0
	}
	span := max(maxX-minX, maxY-minY, 1e-6)
	return xs, ys, ts, 1 / span, (minX + maxX) / 2, (minY + maxY) / 2
}

func pathColumns(tr *telemetry.Track) (x, y, progress points) {
	xs, ys, ts, scale, cx, cy := pathBox(tr)
	if len(xs) < 2 {
		return
	}
	dist := make([]float64, len(xs))
	for i := 1; i < len(xs); i++ {
		dist[i] = dist[i-1] + math.Hypot(xs[i]-xs[i-1], ys[i]-ys[i-1])
	}
	total := max(dist[len(dist)-1], 1e-9)
	for i := range xs {
		x.t, y.t, progress.t = append(x.t, ts[i]), append(y.t, ts[i]), append(progress.t, ts[i])
		x.v = append(x.v, (xs[i]-cx)*scale)
		y.v = append(y.v, (ys[i]-cy)*scale)
		progress.v = append(progress.v, dist[i]/total)
	}
	return
}

// PathPoints returns the whole recording's flight path in the -0.5..0.5 box,
// thinned to at most n points evenly along its distance.
func PathPoints(tr *telemetry.Track, n int) [][2]float64 {
	x, y, prog := pathColumns(tr)
	if len(x.v) < 2 || n < 2 {
		return nil
	}
	var out [][2]float64
	j := 0
	for k := 0; k < n; k++ {
		target := float64(k) / float64(n-1)
		for j+1 < len(prog.v) && prog.v[j+1] < target {
			j++
		}
		out = append(out, [2]float64{x.v[j], y.v[j]})
	}
	return out
}
