package djilog

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Julianlg752/skyhud/internal/telemetry"
)

// Options for FindAndAlign.
type Options struct {
	// Keychains returns the keys for an encrypted log (raw is the file).
	Keychains func(l *Log, raw []byte) ([][]KeyPoint, error)
}

// Result is the log matched to a clip, moved onto the clip's time.
type Result struct {
	Log     string  // file name, "" when none matched
	Offset  float64 // seconds: video time = log time + Offset
	Sticks  telemetry.Series
	Battery telemetry.Series
	Notes   []string
}

const (
	searchBefore = 3 * time.Hour // a log starts at most this long before the clip
	lateStart    = 5.0           // s a log may start after the clip (the goggles record first)
	maxOffset    = 5.0           // s
	offsetStep   = 0.05          // s
	refineWindow = 3.0           // s around the speed or height match where roll may move it
	maxSpread    = 0.1           // s between frames that share one time stamp (OSD runs at 10 Hz)
	sampleStep   = 0.1           // s
	minCorr      = 0.8
	stickCenter  = 1024.0
	stickRange   = 660.0
)

var logName = regexp.MustCompile(`^DJIFlightRecord_(\d{4}-\d{2}-\d{2}_\[\d{2}-\d{2}-\d{2}\])\.txt$`)

// FindAndAlign picks the flight log in folder that belongs to the clip whose
// aircraft data is video (from internal/dji), finds the clock offset by
// matching the log's height with the video's, and returns sticks and battery
// on the video's time.
func FindAndAlign(folder string, video *telemetry.Track, opt Options) (Result, error) {
	entries, err := os.ReadDir(folder)
	if err != nil {
		return Result{}, fmt.Errorf("flight logs folder: %w", err)
	}
	start := video.Start
	// The name is the log's start in the phone's local time, which can be
	// any zone (UTC-12 to UTC+14), so it only narrows the search; the
	// details inside the log give the exact start and end in UTC.
	var candidates []string
	for _, e := range entries {
		m := logName.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		at, err := time.Parse("2006-01-02_[15-04-05]", m[1])
		if err == nil && at.After(start.Add(-searchBefore-12*time.Hour)) && at.Before(start.Add(14*time.Hour+time.Minute)) {
			candidates = append(candidates, e.Name())
		}
	}
	type opened struct {
		name string
		raw  []byte
		log  *Log
	}
	var logs []opened
	for _, name := range candidates {
		raw, err := os.ReadFile(filepath.Join(folder, name))
		if err != nil {
			continue
		}
		l, err := Open(raw)
		if err != nil || !sameAircraft(l.Aircraft, video.Device) {
			continue
		}
		if l.Start.After(start.Add(lateStart*time.Second)) || start.Sub(l.Start) > searchBefore {
			continue
		}
		if l.End.After(l.Start) && l.End.Before(start) {
			continue // the flight ended before the clip
		}
		logs = append(logs, opened{name, raw, l})
	}
	sort.Slice(logs, func(i, j int) bool { return logs[i].log.Start.After(logs[j].log.Start) }) // latest first
	var res Result
	for _, o := range logs {
		r, note := tryLog(o.name, o.raw, o.log, video, opt)
		if note != "" {
			res.Notes = append(res.Notes, note)
		}
		if r != nil {
			r.Log = o.name
			r.Notes = append(res.Notes, r.Notes...)
			return *r, nil
		}
	}
	res.Notes = append(res.Notes, "no matching flight log")
	return res, nil
}

// tryLog returns a result when the log covers the clip and lines up, or a
// note saying why not ("" when it is simply another flight).
func tryLog(name string, raw []byte, l *Log, video *telemetry.Track, opt Options) (*Result, string) {
	start := video.Start
	var kc [][]KeyPoint
	var err error
	if l.Version >= 13 {
		if opt.Keychains == nil {
			return nil, fmt.Sprintf("flight log %s could not be unlocked", name)
		}
		if kc, err = opt.Keychains(l, raw); err != nil {
			return nil, fmt.Sprintf("flight log %s could not be unlocked: %v", name, err)
		}
	}
	frames, err := l.Frames(kc)
	if err != nil || len(frames) == 0 {
		return nil, ""
	}
	// Log times relative to the clip start.
	times := frameTimes(frames, start)
	var lt, lh, ls []float64
	for i, f := range frames {
		if math.IsNaN(times[i]) {
			continue
		}
		lt = append(lt, times[i])
		lh = append(lh, f.Height)
		ls = append(ls, math.Hypot(f.XSpeed, f.YSpeed))
	}
	if len(lt) == 0 || lt[0] > lateStart || lt[len(lt)-1] < video.Duration/2 {
		return nil, "" // doesn't cover the clip
	}
	// Ground speed has sharper turns than height, and the video's height can
	// drift against the log's, so speed sets the offset when both have it.
	vc, lc := video.Channels[telemetry.Height], lh
	if vs := video.Channels[telemetry.Speed]; len(vs.T) > 1 && varies(ls) {
		vc, lc = vs, ls
	}
	offset, corr := bestOffset(vc.T, column0(vc.V), lt, lc, video.Duration)
	if corr < minCorr {
		return nil, fmt.Sprintf("flight log %s does not line up with the video (match %.2f)", name, corr)
	}
	if math.Abs(offset) > maxOffset-offsetStep/2 {
		return nil, fmt.Sprintf("flight log %s could not be lined up: the best match is %.0f s off or more", name, maxOffset)
	}
	offset = refineByRoll(offset, video, frames, times, vc, lt, lc)
	r := &Result{Offset: offset}
	sticks := hasStickData(frames)
	if !sticks {
		r.Notes = append(r.Notes, "flight log has no stick positions (motion controller?)")
	}
	for i, f := range frames {
		if math.IsNaN(times[i]) {
			continue
		}
		at := times[i] + offset
		if at < -1 || at > video.Duration+1 {
			continue
		}
		if sticks && f.HasRC {
			r.Sticks.T = append(r.Sticks.T, at)
			r.Sticks.V = append(r.Sticks.V, []float64{stick(f.Throttle), stick(f.Rudder), stick(f.Elevator), stick(f.Aileron)})
		}
		if f.Battery > 0 || f.Volts > 0 {
			r.Battery.T = append(r.Battery.T, at)
			r.Battery.V = append(r.Battery.V, []float64{float64(f.Battery), f.Volts})
		}
	}
	return r, ""
}

// hasStickData reports whether the log holds real stick movements: RC
// records whose channels move by more than a nudge somewhere in the flight.
func hasStickData(frames []Frame) bool {
	const minTravel = 200 // of about 1320 from end to end
	lo, hi := [4]int{1 << 30, 1 << 30, 1 << 30, 1 << 30}, [4]int{}
	seen := false
	for _, f := range frames {
		if !f.HasRC {
			continue
		}
		seen = true
		for k, v := range [4]int{f.Throttle, f.Rudder, f.Elevator, f.Aileron} {
			lo[k], hi[k] = min(lo[k], v), max(hi[k], v)
		}
	}
	if !seen {
		return false
	}
	for k := range lo {
		if hi[k]-lo[k] > minTravel {
			return true
		}
	}
	return false
}

func sameAircraft(log, device string) bool {
	norm := func(s string) string { return strings.ToLower(strings.ReplaceAll(s, " ", "")) }
	return log != "" && device != "" && norm(log) == norm(device)
}

func stick(v int) float64 { return math.Max(-1, math.Min(1, (float64(v)-stickCenter)/stickRange)) }

func column0(v [][]float64) []float64 {
	out := make([]float64, len(v))
	for i, r := range v {
		if len(r) > 0 {
			out[i] = r[0]
		}
	}
	return out
}

// bestOffset finds the shift o (video time = log time + o) that best
// correlates the log's height with the video's over the clip.
func bestOffset(vt, vh, lt, lh []float64, duration float64) (float64, float64) {
	if len(vt) < 2 || len(lt) < 2 {
		return 0, 0
	}
	grid := offsetGrid(duration)
	vs := sample(vt, vh, grid, 0)
	best, bestCorr := 0.0, -2.0
	for o := -maxOffset; o <= maxOffset+1e-9; o += offsetStep {
		if c := corr(vs, sample(lt, lh, grid, o)); c > bestCorr {
			best, bestCorr = math.Round(o/offsetStep)*offsetStep, c
		}
	}
	return best, bestCorr
}

// refineByRoll moves the offset found from speed or height (whose match can
// be broad and sit a second or more off) to where the video's roll best
// follows the aileron stick, within refineWindow of it. Without sticks or
// orientation it returns the offset unchanged.
func refineByRoll(offset float64, video *telemetry.Track, frames []Frame, times []float64, vc telemetry.Series, lt, lc []float64) float64 {
	o := video.Channels[telemetry.Orientation]
	if len(o.T) < 2 || !hasStickData(frames) {
		return offset
	}
	var rt, rv []float64
	step := max(1, len(o.T)/(int(video.Duration/sampleStep)*4+1)) // a few points per grid step is plenty
	for i := 0; i < len(o.T); i += step {
		if q := o.V[i]; len(q) >= 4 {
			w, x, y, z := q[0], q[1], q[2], q[3]
			rt = append(rt, o.T[i])
			rv = append(rv, math.Atan2(2*(w*x+y*z), 1-2*(x*x+y*y)))
		}
	}
	var at, av []float64
	for i, f := range frames {
		if !math.IsNaN(times[i]) && f.HasRC {
			at = append(at, times[i])
			av = append(av, stick(f.Aileron))
		}
	}
	if len(rt) < 2 || len(at) < 2 {
		return offset
	}
	grid := offsetGrid(video.Duration)
	vs, rs := sample(vc.T, column0(vc.V), grid, 0), sample(rt, rv, grid, 0)
	best, bestScore := offset, -4.0
	for o := offset - refineWindow; o <= offset+refineWindow+1e-9; o += offsetStep {
		// Roll and stick can be opposite in sign; the size of the match counts.
		score := corr(vs, sample(lt, lc, grid, o)) + math.Abs(corr(rs, sample(at, av, grid, o)))
		if score > bestScore {
			best, bestScore = math.Round(o/offsetStep)*offsetStep, score
		}
	}
	return best
}

// frameTimes returns each frame's time in seconds from start (NaN when it
// has none). The OSD's flight time counter gives the steps between frames
// when the log has it; the time stamps, which can lag by seconds and are
// sometimes years off, only place it (their median). Without the counter,
// a run of frames sharing one stamp is spread evenly up to the next stamp,
// at most maxSpread apart.
func frameTimes(frames []Frame, start time.Time) []float64 {
	if out, ok := flyTimes(frames, start); ok {
		return out
	}
	out := make([]float64, len(frames))
	var run []int // frames sharing the current stamp
	flush := func(next float64) {
		if len(run) == 0 {
			return
		}
		t0 := out[run[0]]
		step := maxSpread
		if !math.IsNaN(next) {
			step = min(maxSpread, (next-t0)/float64(len(run)))
		}
		for j, i := range run {
			out[i] = t0 + float64(j)*step
		}
		run = run[:0]
	}
	for i, f := range frames {
		if f.Time.IsZero() {
			out[i] = math.NaN()
			continue
		}
		out[i] = f.Time.Sub(start).Seconds()
		if len(run) > 0 && out[i] != out[run[0]] {
			flush(out[i])
		}
		run = append(run, i)
	}
	flush(math.NaN())
	return out
}

// flyTimes places the flight time counter on the clip's clock; ok is false
// when a frame lacks the counter or it runs backwards.
func flyTimes(frames []Frame, start time.Time) ([]float64, bool) {
	var d []float64
	for i, f := range frames {
		if f.FlyTime < 0 || i > 0 && f.FlyTime < frames[i-1].FlyTime {
			return nil, false
		}
		if !f.Time.IsZero() {
			d = append(d, f.Time.Sub(start).Seconds()-f.FlyTime)
		}
	}
	if len(d) == 0 {
		return nil, false
	}
	sort.Float64s(d)
	c := d[len(d)/2]
	out := make([]float64, len(frames))
	for i, f := range frames {
		out[i] = f.FlyTime + c
	}
	return out, true
}

// offsetGrid is the clip's times the matches are compared at.
func offsetGrid(duration float64) []float64 {
	var grid []float64
	for at := 0.0; at <= duration; at += sampleStep {
		grid = append(grid, at)
	}
	return grid
}

// sample returns v at each grid time moved back by o.
func sample(t, v, grid []float64, o float64) []float64 {
	out := make([]float64, len(grid))
	for i, at := range grid {
		out[i] = interp(t, v, at-o)
	}
	return out
}

// varies reports whether v holds more than one value.
func varies(v []float64) bool {
	for _, x := range v {
		if x != v[0] {
			return true
		}
	}
	return false
}

func interp(t, v []float64, at float64) float64 {
	j := sort.SearchFloat64s(t, at)
	switch {
	case j == 0:
		return v[0]
	case j >= len(t):
		return v[len(v)-1]
	case t[j] == t[j-1]:
		return v[j]
	}
	f := (at - t[j-1]) / (t[j] - t[j-1])
	return v[j-1] + f*(v[j]-v[j-1])
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
	if da == 0 || db == 0 {
		return 0
	}
	return num / math.Sqrt(da*db)
}
