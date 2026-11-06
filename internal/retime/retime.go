// Package retime maps time in a timeline shot to time in its source file,
// following speed changes and speed ramps.
package retime

// Key is one point of a speed ramp: at Clip seconds of retimed time the shot
// shows Source seconds of the file. Both count from the start of the media,
// as in the "Key Frames" of Resolve's OTIO export.
type Key struct {
	Clip, Source float64
}

// Map gives the source time shown at each moment of a shot. Start is the
// source time on the shot's first frame. Without a ramp, t seconds into the
// shot show Start + t*Speed (1 = normal, 0.5 = half speed, 0 = freeze).
// With two or more Keys the map runs through them in straight lines and
// Speed is ignored.
type Map struct {
	Start float64
	Speed float64
	Keys  []Key
}

// Ramped reports whether the map follows a speed ramp.
func (m Map) Ramped() bool { return len(m.Keys) >= 2 }

// SourceAt returns the source time shown t seconds into the shot.
func (m Map) SourceAt(t float64) float64 {
	if !m.Ramped() {
		return m.Start + t*m.Speed
	}
	return m.interp(m.firstClip() + t)
}

// firstClip finds the clip time of the shot's first frame: the first point,
// in key order, where the map shows Start.
func (m Map) firstClip() float64 {
	k := m.Keys
	for i := 0; i+1 < len(k); i++ {
		a, b := k[i], k[i+1]
		lo, hi := min(a.Source, b.Source), max(a.Source, b.Source)
		if m.Start < lo || m.Start > hi {
			continue
		}
		if a.Source == b.Source {
			return a.Clip
		}
		return a.Clip + (m.Start-a.Source)*(b.Clip-a.Clip)/(b.Source-a.Source)
	}
	// Outside every segment: extend the end segment that points toward Start.
	first, last := segment(k, 0), segment(k, len(k)-2)
	for _, s := range [][2]Key{first, last} {
		if s[1].Source != s[0].Source {
			return s[0].Clip + (m.Start-s[0].Source)*(s[1].Clip-s[0].Clip)/(s[1].Source-s[0].Source)
		}
	}
	if abs(m.Start-k[0].Source) <= abs(m.Start-k[len(k)-1].Source) {
		return k[0].Clip
	}
	return k[len(k)-1].Clip
}

// interp evaluates the map at clip time x, extending the end segments.
func (m Map) interp(x float64) float64 {
	k := m.Keys
	i := 0
	for i+2 < len(k) && x > k[i+1].Clip {
		i++
	}
	s := segment(k, i)
	if s[1].Clip == s[0].Clip {
		return s[1].Source
	}
	return s[0].Source + (x-s[0].Clip)*(s[1].Source-s[0].Source)/(s[1].Clip-s[0].Clip)
}

func segment(k []Key, i int) [2]Key { return [2]Key{k[i], k[i+1]} }

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
