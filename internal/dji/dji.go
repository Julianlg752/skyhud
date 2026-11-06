// Package dji reads the flight data that the DJI Avata 2 camera writes into
// its video files (the "djmd" track: one protobuf packet per frame).
//
// DJI doesn't document the format. The field map below was worked out on one
// Avata 2 clip by comparing it with the decoded flight log (correlation about
// 0.999).
//
//	.1.1.10        device name ("DJI Avata2"); .1.1.2 firmware
//	.3.3.2.3       orientation quaternions w, x, y, z (about 2000 per second)
//	.3.4.2.1-3     velocity north, east, down (m/s)
//	.3.4.4.1.2-3   latitude, longitude (radians)
//	.3.4.5.1       height above takeoff (mm)
//
// Serial numbers (.1.1.5 and others) are never read.
package dji

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/Julianlg752/skyhud/internal/mp4"
	"github.com/Julianlg752/skyhud/internal/telemetry"
)

// ErrNoTelemetry means the file has no djmd track.
var ErrNoTelemetry = errors.New("no DJI flight data in this file")

const (
	earthRadius = 6371008.8 // m
	standardG   = 9.80665   // m/s^2
	maxSpeed    = 60.0      // m/s; anything faster is a decoding error
	maxHeight   = 2000.0    // m
	gSmoothing  = 0.5       // s; velocity is coarse, so smooth it before differentiating
)

// Read decodes every packet of the file's djmd track.
func Read(path string) (*telemetry.Track, error) {
	f, err := mp4.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tr, ok := f.TrackByFormat("djmd")
	if !ok {
		return nil, ErrNoTelemetry
	}
	out := &telemetry.Track{Source: "dji", Start: f.CreationTime, Duration: f.Duration, Channels: map[string]telemetry.Series{}}
	r := reader{track: out}
	for _, s := range tr.Samples {
		b, err := f.ReadSample(s)
		if err != nil {
			return nil, err
		}
		r.packet(b, s.Start, s.Duration)
	}
	r.finish()
	return out, nil
}

type reader struct {
	track                     *telemetry.Track
	firmware                  string
	origin                    *[2]float64
	quats, vels, heights, pos int
}

func (r *reader) packet(b []byte, start, dur float64) {
	t := r.track
	for _, h := range all(b, 1, 1) {
		if t.Device == "" {
			t.Device = str(h, 10)
		}
		if r.firmware == "" {
			r.firmware = str(h, 2)
		}
	}

	quats := all(b, 3, 3, 2, 3)
	for i, m := range quats {
		s := scalars(m)
		q := [4]float64{s[1].f32(), s[2].f32(), s[3].f32(), s[4].f32()}
		n := math.Sqrt(q[0]*q[0] + q[1]*q[1] + q[2]*q[2] + q[3]*q[3])
		if !(n > 0.9 && n < 1.1) { // also rejects NaN
			continue
		}
		t.Add(telemetry.Orientation, start+dur*float64(i)/float64(len(quats)), q[:]...)
		r.quats++
	}

	for _, fl := range all(b, 3, 4) {
		for _, m := range all(fl, 2) {
			s := scalars(m)
			var v [3]float64
			readable, any := true, false
			for k := range 3 {
				x, present, ok := float32Field(s, uint64(k+1))
				v[k], readable, any = x, readable && ok, any || present
			}
			if !readable || !any && len(m) > 0 { // retyped, or moved to other field numbers
				continue
			}
			norm := math.Sqrt(v[0]*v[0] + v[1]*v[1] + v[2]*v[2])
			if !(norm < maxSpeed) {
				continue
			}
			t.Add(telemetry.Velocity, start, v[:]...)
			t.Add(telemetry.Speed, start, math.Hypot(v[0], v[1]))
			t.Add(telemetry.Speed3D, start, norm)
			r.vels++
		}
		for _, m := range all(fl, 5) {
			// protobuf leaves out zero values, so a height message without
			// field 1 means 0 m (on the ground before takeoff).
			mm, _, ok := float32Field(scalars(m), 1)
			h := mm / 1000
			if !ok || !(math.Abs(h) < maxHeight) {
				continue
			}
			t.Add(telemetry.Height, start, h)
			r.heights++
		}
		for _, m := range all(fl, 4, 1) {
			s := scalars(m)
			lat, lon := s[2].f64(), s[3].f64()
			if !(math.Abs(lat) <= math.Pi/2 && math.Abs(lon) <= math.Pi) || lat == 0 && lon == 0 {
				continue
			}
			r.position(start, lat*180/math.Pi, lon*180/math.Pi)
			r.pos++
		}
	}
}

func (r *reader) position(at, lat, lon float64) {
	if r.origin == nil {
		r.origin = &[2]float64{lat, lon}
	}
	lat0 := r.origin[0] * math.Pi / 180
	x := (lon - r.origin[1]) * math.Pi / 180 * math.Cos(lat0) * earthRadius
	y := (lat - r.origin[0]) * math.Pi / 180 * earthRadius
	r.track.Add(telemetry.GPS, at, lat, lon)
	r.track.Add(telemetry.Path, at, x, y)
}

func (r *reader) finish() {
	t := r.track
	if t.Device != "" && strings.ToLower(strings.ReplaceAll(t.Device, " ", "")) != "djiavata2" {
		t.Notes = append(t.Notes, fmt.Sprintf("the djmd field map was only checked on the DJI Avata 2; values from %s may be wrong", t.Device))
	}
	// A channel that is zero on every packet more likely moved than stayed
	// at zero for the whole clip: show it as missing.
	if allZero(t.Channels[telemetry.Velocity]) {
		for _, c := range []string{telemetry.Velocity, telemetry.Speed, telemetry.Speed3D} {
			delete(t.Channels, c)
		}
		if r.vels > 0 {
			t.Notes = append(t.Notes, "djmd velocity is always zero in this clip; speed is treated as missing")
		}
		r.vels = -1
	}
	if allZero(t.Channels[telemetry.Height]) {
		delete(t.Channels, telemetry.Height)
		if r.heights > 0 {
			t.Notes = append(t.Notes, "djmd height is always zero in this clip; height is treated as missing")
		}
		r.heights = -1
	}
	for _, c := range []struct {
		n           int
		field, what string
	}{
		{r.quats, ".3.3.2.3", "orientation"},
		{r.vels, ".3.4.2", "velocity"},
		{r.heights, ".3.4.5.1", "height"},
		{r.pos, ".3.4.4.1", "position"},
	} {
		if c.n == 0 {
			t.Notes = append(r.track.Notes, fmt.Sprintf("djmd field %s (%s) missing or unreadable; firmware %s", c.field, c.what, r.firmware))
		}
	}
	r.estimateG()
}

// estimateG derives g-force from velocity changes: the size of
// (acceleration - gravity) in g, with gravity pointing down. It is an
// estimate, so the series is marked as such.
func (r *reader) estimateG() {
	v := r.track.Channels[telemetry.Velocity]
	if len(v.T) < 3 {
		return
	}
	smooth := make([][3]float64, len(v.T))
	lo, hi := 0, 0
	var sum [3]float64
	for i, t := range v.T {
		for hi < len(v.T) && v.T[hi] <= t+gSmoothing/2 {
			for k := range 3 {
				sum[k] += v.V[hi][k]
			}
			hi++
		}
		for v.T[lo] < t-gSmoothing/2 {
			for k := range 3 {
				sum[k] -= v.V[lo][k]
			}
			lo++
		}
		for k := range 3 {
			smooth[i][k] = sum[k] / float64(hi-lo)
		}
	}
	g := telemetry.Series{Estimated: true}
	for i := 1; i+1 < len(v.T); i++ {
		dt := v.T[i+1] - v.T[i-1]
		if dt <= 0 {
			continue
		}
		var a [3]float64
		for k := range 3 {
			a[k] = (smooth[i+1][k] - smooth[i-1][k]) / dt
		}
		a[2] -= standardG // NED: gravity points down (+z)
		g.T = append(g.T, v.T[i])
		g.V = append(g.V, []float64{math.Sqrt(a[0]*a[0]+a[1]*a[1]+a[2]*a[2]) / standardG})
	}
	r.track.Channels[telemetry.GForce] = g
}

// str returns string field num of message m.
func str(m []byte, num uint64) string {
	fs, err := parse(m)
	if err != nil {
		return ""
	}
	for _, f := range fs {
		if f.num == num && f.wire == 2 {
			return string(f.b)
		}
	}
	return ""
}

func allZero(s telemetry.Series) bool {
	for _, row := range s.V {
		for _, x := range row {
			if x != 0 {
				return false
			}
		}
	}
	return len(s.V) > 0
}
