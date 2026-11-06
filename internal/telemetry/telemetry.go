// Package telemetry holds flight data in one shape for every source, and
// writes the HUD data file that the Resolve side reads.
package telemetry

import (
	"encoding/json"
	"fmt"
	"io"
	"time"
)

// Channel names and their values. Times are seconds from the start of the video.
const (
	Speed       = "speed"       // [m/s] ground speed
	Speed3D     = "speed3d"     // [m/s]
	Altitude    = "altitude"    // [m] GPS altitude
	Path        = "path"        // [x east m, y north m] from the first GPS fix
	GPS         = "gps"         // [lat deg, lon deg]; only written with keepLocation
	Accel       = "accel"       // [x, y, z m/s^2] in the source's axis order
	Gyro        = "gyro"        // [x, y, z rad/s] in the source's axis order
	Gravity     = "gravity"     // [x, y, z] unit vector
	Orientation = "orientation" // [w, x, y, z] quaternion
	GForce      = "gforce"      // [g]
	Height      = "height"      // [m] above takeoff, from the aircraft
	Velocity    = "velocity"    // [north, east, down m/s]
	Sticks      = "sticks"      // [throttle, yaw, pitch, roll], -1..1
	Battery     = "battery"     // [percent, volts]
)

// Series is one channel: a time for each row of values.
type Series struct {
	T         []float64   `json:"t"`
	V         [][]float64 `json:"v"`
	Estimated bool        `json:"estimated"`
}

// Track is everything read from one clip.
type Track struct {
	Source   string
	Device   string
	Start    time.Time
	Duration float64
	Channels map[string]Series
	Notes    []string
	NoGPS    [][2]float64 // spans in seconds with no GPS lock
}

// AllNotes returns the notes plus one line per span without GPS lock.
func (t *Track) AllNotes() []string {
	out := append([]string{}, t.Notes...)
	for _, g := range t.NoGPS {
		out = append(out, fmt.Sprintf("no GPS lock from %.1f s to %.1f s", g[0], g[1]))
	}
	return out
}

// Add appends one row to a channel.
func (t *Track) Add(name string, at float64, v ...float64) {
	if t.Channels == nil {
		t.Channels = map[string]Series{}
	}
	s := t.Channels[name]
	s.T = append(s.T, at)
	s.V = append(s.V, v)
	t.Channels[name] = s
}

type hudFile struct {
	Format   string            `json:"format"`
	Version  int               `json:"version"`
	Source   string            `json:"source"`
	Device   string            `json:"device"`
	Start    string            `json:"start"`
	Duration float64           `json:"duration"`
	Notes    []string          `json:"notes"`
	Channels map[string]Series `json:"channels"`
}

// WriteHUDFile writes the track as a HUD data file. Absolute GPS positions are
// left out unless keepLocation is true.
func (t *Track) WriteHUDFile(w io.Writer, keepLocation bool) error {
	ch := make(map[string]Series, len(t.Channels))
	for name, s := range t.Channels {
		if name == GPS && !keepLocation {
			continue
		}
		ch[name] = s
	}
	notes := t.AllNotes()
	f := hudFile{
		Format:   "skyhud-hud",
		Version:  1,
		Source:   t.Source,
		Device:   t.Device,
		Start:    t.Start.UTC().Format(time.RFC3339),
		Duration: t.Duration,
		Notes:    notes,
		Channels: ch,
	}
	return json.NewEncoder(w).Encode(f)
}
