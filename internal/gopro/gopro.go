// Package gopro reads the GPMF telemetry track of a GoPro MP4 into a
// telemetry.Track.
package gopro

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/Julianlg752/skyhud/internal/gpmf"
	"github.com/Julianlg752/skyhud/internal/mp4"
	"github.com/Julianlg752/skyhud/internal/telemetry"
)

// ErrNoTelemetry means the file has no GoPro telemetry track, as with
// stabilized exports from Gyroflow.
var ErrNoTelemetry = errors.New("no GoPro telemetry in this file")

const (
	earthRadius = 6371008.8 // m
	standardG   = 9.80665   // m/s^2
)

// Read decodes every telemetry sample in the file.
func Read(path string) (*telemetry.Track, error) {
	f, err := mp4.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tr, ok := f.TrackByFormat("gpmd")
	if !ok {
		return nil, ErrNoTelemetry
	}

	out := &telemetry.Track{
		Source:   "gopro",
		Start:    f.CreationTime,
		Duration: f.Duration,
		Channels: map[string]telemetry.Series{},
	}
	r := reader{track: out}
	for _, s := range tr.Samples {
		b, err := f.ReadSample(s)
		if err != nil {
			return nil, err
		}
		p, err := gpmf.Decode(b)
		if err != nil {
			return nil, fmt.Errorf("telemetry at %.1f s: %w", s.Start, err)
		}
		if out.Device == "" {
			out.Device = p.Device
		}
		r.sample(p, s.Start, s.Duration)
	}
	r.finish()
	if note := chapterNote(path); note != "" {
		out.Notes = append(out.Notes, note)
	}
	return out, nil
}

// GoPro splits long recordings into chapters: GX010123.MP4, GX020123.MP4, ...
var chapterName = regexp.MustCompile(`^(G[A-Z])(\d{2})(\d{4})(\.[Mm][Pp]4)$`)

// chapterNote says when a clip is part of a longer recording: chapter 2 or
// later by its name alone, chapter 1 when the next chapter sits next to it.
func chapterNote(path string) string {
	m := chapterName.FindStringSubmatch(filepath.Base(path))
	if m == nil {
		return ""
	}
	n, _ := strconv.Atoi(m[2])
	if n >= 2 {
		return fmt.Sprintf("this clip is chapter %d of a longer recording that starts with %s01%s%s", n, m[1], m[3], strings.ToUpper(m[4]))
	}
	if next := nextChapter(path); next != "" {
		return fmt.Sprintf("this clip is chapter 1 of a longer recording; %s continues it", next)
	}
	return ""
}

// nextChapter returns the next chapter's file name if it sits next to path.
func nextChapter(path string) string {
	m := chapterName.FindStringSubmatch(filepath.Base(path))
	if m == nil {
		return ""
	}
	n, _ := strconv.Atoi(m[2])
	for _, ext := range []string{m[4], ".MP4", ".mp4"} {
		name := fmt.Sprintf("%s%02d%s%s", m[1], n+1, m[3], ext)
		if _, err := os.Stat(filepath.Join(filepath.Dir(path), name)); err == nil {
			return name
		}
	}
	return ""
}

type gap struct{ from, to float64 }

type reader struct {
	track    *telemetry.Track
	origin   *[2]float64 // lat, lon of the first fix
	unlocked []gap
	orin     string
}

// spread returns the time of row i when n rows cover [start, start+dur).
func spread(start, dur float64, i, n int) float64 {
	return start + dur*float64(i)/float64(n)
}

func (r *reader) sample(p gpmf.Payload, start, dur float64) {
	fix := -1.0
	for _, s := range p.Streams {
		if s.Key == "GPSF" && len(s.Rows) > 0 {
			fix = s.Rows[0][0]
		}
	}
	gpsSeen, gpsUsed := false, false
	for _, s := range p.Streams {
		n := len(s.Rows)
		switch s.Key {
		case "GPS5", "GPS9":
			gpsSeen = true
			for i, row := range s.Rows {
				rowFix := fix
				if s.Key == "GPS9" && len(row) >= 9 {
					rowFix = row[8]
				}
				if !hasFix(rowFix, row) {
					continue
				}
				gpsUsed = true
				r.gps(spread(start, dur, i, n), row)
			}
		case "ACCL":
			r.noteOrin(s.Orin)
			for i, row := range s.Rows {
				if len(row) < 3 {
					continue
				}
				at := spread(start, dur, i, n)
				r.track.Add(telemetry.Accel, at, row[0], row[1], row[2])
				r.track.Add(telemetry.GForce, at, math.Sqrt(row[0]*row[0]+row[1]*row[1]+row[2]*row[2])/standardG)
			}
		case "GYRO":
			r.add(telemetry.Gyro, s, start, dur, 3)
		case "GRAV":
			r.add(telemetry.Gravity, s, start, dur, 3)
		case "CORI":
			r.add(telemetry.Orientation, s, start, dur, 4)
		}
	}
	if gpsSeen && !gpsUsed {
		r.unlocked = append(r.unlocked, gap{start, start + dur})
	}
}

// hasFix accepts a 2D or 3D fix; without a fix value, any nonzero position counts.
func hasFix(fix float64, row []float64) bool {
	if len(row) < 5 {
		return false
	}
	if fix >= 0 {
		return fix >= 2
	}
	return row[0] != 0 || row[1] != 0
}

func (r *reader) gps(at float64, row []float64) {
	lat, lon := row[0], row[1]
	if r.origin == nil {
		r.origin = &[2]float64{lat, lon}
	}
	lat0 := r.origin[0] * math.Pi / 180
	x := (lon - r.origin[1]) * math.Pi / 180 * math.Cos(lat0) * earthRadius
	y := (lat - r.origin[0]) * math.Pi / 180 * earthRadius
	r.track.Add(telemetry.GPS, at, lat, lon)
	r.track.Add(telemetry.Path, at, x, y)
	r.track.Add(telemetry.Altitude, at, row[2])
	r.track.Add(telemetry.Speed, at, row[3])
	r.track.Add(telemetry.Speed3D, at, row[4])
}

func (r *reader) add(name string, s gpmf.Stream, start, dur float64, cols int) {
	for i, row := range s.Rows {
		if len(row) >= cols {
			r.track.Add(name, spread(start, dur, i, len(s.Rows)), row[:cols]...)
		}
	}
}

func (r *reader) noteOrin(orin string) {
	if orin != "" && r.orin == "" {
		r.orin = orin
		r.track.Notes = append(r.track.Notes, fmt.Sprintf("accelerometer and gyro axis order (ORIN): %s", orin))
	}
}

func (r *reader) finish() {
	var merged []gap
	for _, g := range r.unlocked {
		if len(merged) > 0 && g.from-merged[len(merged)-1].to < 0.01 {
			merged[len(merged)-1].to = g.to
			continue
		}
		merged = append(merged, g)
	}
	for _, g := range merged {
		r.track.NoGPS = append(r.track.NoGPS, [2]float64{g.from, g.to})
	}
}
