package otio

import (
	"bytes"
	"encoding/json"
	"math"
	"strings"
	"testing"
)

const ntsc = 30000.0 / 1001

func rt(v, rate float64) map[string]any {
	return map[string]any{"OTIO_SCHEMA": "RationalTime.1", "value": v, "rate": rate}
}

func rng(start, dur, rate float64) map[string]any {
	return map[string]any{"OTIO_SCHEMA": "TimeRange.1", "start_time": rt(start, rate), "duration": rt(dur, rate)}
}

// clip is shaped like a clip in Resolve's OTIO export. The media starts at
// timecode frame 1476247, as in the real test clip.
func clip(path string, inFrame, frames, rate float64, effects ...map[string]any) map[string]any {
	const media = 1476247.0
	fx := []any{map[string]any{"OTIO_SCHEMA": "Effect.1", "name": "", "effect_name": "Resolve Effect", "metadata": map[string]any{}}}
	for _, e := range effects {
		fx = append(fx, e)
	}
	return map[string]any{
		"OTIO_SCHEMA":  "Clip.2",
		"name":         path[strings.LastIndex(path, "/")+1:],
		"source_range": rng(media+inFrame, frames, rate),
		"effects":      fx,
		"media_references": map[string]any{"DEFAULT_MEDIA": map[string]any{
			"OTIO_SCHEMA":     "ExternalReference.1",
			"target_url":      path,
			"available_range": rng(media, 1999, rate),
		}},
		"active_media_reference_key": "DEFAULT_MEDIA",
	}
}

func gap(frames, rate float64) map[string]any {
	return map[string]any{"OTIO_SCHEMA": "Gap.1", "source_range": rng(0, frames, rate)}
}

func warp(scalar float64) map[string]any {
	return map[string]any{"OTIO_SCHEMA": "LinearTimeWarp.1", "time_scalar": scalar, "metadata": map[string]any{"Resolve_OTIO": map[string]any{"Retime Mode": "NONE"}}}
}

func ramp(curved bool, keys ...[2]float64) map[string]any {
	var kf []any
	for _, k := range keys {
		kf = append(kf, []any{k[0], k[1], curved, 0.0, 0.0, 0.0, 0.0})
	}
	return map[string]any{"OTIO_SCHEMA": "TimeEffect.1", "metadata": map[string]any{"Resolve_OTIO": map[string]any{"Key Frames": kf, "Retime Mode": "NONE", "Speed Ratio": 0.85}}}
}

func track(kind string, children ...any) map[string]any {
	return map[string]any{"OTIO_SCHEMA": "Track.1", "name": kind + " 1", "kind": kind, "children": children}
}

func otioDoc(t *testing.T, rate float64, start int64, videoTracks ...[]any) []byte {
	t.Helper()
	var tracks []any
	for _, v := range videoTracks {
		tracks = append(tracks, track("Video", v...))
	}
	tracks = append(tracks, track("Audio", gap(100, rate)))
	doc := map[string]any{
		"OTIO_SCHEMA":       "Timeline.1",
		"name":              "test",
		"global_start_time": rt(float64(start), rate),
		"tracks":            map[string]any{"OTIO_SCHEMA": "Stack.1", "children": tracks},
	}
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func read(t *testing.T, b []byte) *Timeline {
	t.Helper()
	tl, err := Read(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	return tl
}

func near(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-6 {
		t.Errorf("%s = %v, want %v", name, got, want)
	}
}

func TestReadPlainAndConstant(t *testing.T) {
	tl := read(t, otioDoc(t, ntsc, 108000, []any{
		clip("/clips/GX010123.MP4", 0, 299, ntsc),
		clip("/clips/GX010123.MP4", 600, 598, ntsc, warp(0.5)),
	}))
	if len(tl.Clips) != 2 {
		t.Fatalf("clips = %+v", tl.Clips)
	}
	a, b := tl.Clips[0], tl.Clips[1]
	if a.RecordStart != 108000 || a.Frames != 299 || b.RecordStart != 108299 || b.Frames != 598 {
		t.Fatalf("record ranges: %+v %+v", a, b)
	}
	if a.Track != 1 || a.Path != "/clips/GX010123.MP4" || a.Name != "GX010123.MP4" {
		t.Fatalf("clip a = %+v", a)
	}
	near(t, "a start", a.Map.Start, 0)
	near(t, "a speed", a.Map.Speed, 1)
	near(t, "b start", b.Map.Start, 600/ntsc)
	near(t, "b speed", b.Map.Speed, 0.5)
	if b.Map.Ramped() {
		t.Fatal("constant speed read as a ramp")
	}
}

func TestReadRamp(t *testing.T) {
	tl := read(t, otioDoc(t, ntsc, 108000, []any{
		clip("/clips/a.MP4", 200, 400, ntsc, ramp(false, [2]float64{0, 0}, [2]float64{3.3366, 0.8342}, [2]float64{10.01, 7.5075})),
	}))
	m := tl.Clips[0].Map
	if !m.Ramped() || len(m.Keys) != 3 {
		t.Fatalf("map = %+v", m)
	}
	near(t, "key 1 clip", m.Keys[1].Clip, 3.3366)
	near(t, "key 1 source", m.Keys[1].Source, 0.8342)
	near(t, "start", m.Start, 200/ntsc)
	for _, n := range tl.Clips[0].Notes {
		if strings.Contains(n, "smooth") {
			t.Fatalf("straight ramp got note %q", n)
		}
	}
}

func TestReadGapsTransitionsAndTracks(t *testing.T) {
	tl := read(t, otioDoc(t, ntsc, 108000,
		[]any{
			gap(50, ntsc),
			clip("/clips/a.MP4", 0, 100, ntsc),
			map[string]any{"OTIO_SCHEMA": "Transition.1", "in_offset": rt(12, ntsc), "out_offset": rt(12, ntsc)},
			clip("/clips/b.MP4", 0, 100, ntsc),
		},
		[]any{gap(10, ntsc), clip("/clips/c.MP4", 0, 30, ntsc)},
	))
	got := map[string][2]int64{}
	for _, c := range tl.Clips {
		got[c.Name] = [2]int64{int64(c.Track), c.RecordStart}
	}
	want := map[string][2]int64{"a.MP4": {1, 108050}, "b.MP4": {1, 108150}, "c.MP4": {2, 108010}}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: track/start = %v, want %v", k, got[k], v)
		}
	}
	if len(tl.Clips) != 3 {
		t.Fatalf("clips = %d, want 3 (audio tracks ignored)", len(tl.Clips))
	}
}

func TestReadCurvedNote(t *testing.T) {
	tl := read(t, otioDoc(t, ntsc, 0, []any{
		clip("/clips/a.MP4", 0, 100, ntsc, ramp(true, [2]float64{0, 0}, [2]float64{2, 1})),
	}))
	notes := strings.Join(tl.Clips[0].Notes, "|")
	if !strings.Contains(notes, "smooth speed ramp: the HUD follows it as straight lines") {
		t.Fatalf("notes = %q", notes)
	}
}

func TestReadCompoundSkipped(t *testing.T) {
	compound := map[string]any{"OTIO_SCHEMA": "Stack.1", "name": "Compound Clip 1", "source_range": rng(0, 120, ntsc), "children": []any{}}
	tl := read(t, otioDoc(t, ntsc, 1000, []any{compound, clip("/clips/a.MP4", 0, 10, ntsc)}))
	if len(tl.Clips) != 1 || tl.Clips[0].RecordStart != 1120 {
		t.Fatalf("clips = %+v", tl.Clips)
	}
	if len(tl.Notes) == 0 || !strings.Contains(tl.Notes[0], "compound") {
		t.Fatalf("notes = %q", tl.Notes)
	}
}

func TestFind(t *testing.T) {
	tl := read(t, otioDoc(t, ntsc, 108000, []any{clip("/clips/a.MP4", 0, 10, ntsc), clip("/clips/b.MP4", 0, 10, ntsc)}))
	if c, ok := tl.Find(1, 108010); !ok || c.Name != "b.MP4" {
		t.Fatalf("Find(1, 108010) = %+v %v", c, ok)
	}
	if _, ok := tl.Find(1, 108011); ok {
		t.Fatal("Find matched one frame off")
	}
	if _, ok := tl.Find(2, 108000); ok {
		t.Fatal("Find matched the wrong track")
	}
}

func TestReadGarbage(t *testing.T) {
	for _, in := range []string{"not json", `{"OTIO_SCHEMA": "Timeline.1"}`, `{"tracks": {"children": [1, "x", null]}}`, ""} {
		if _, err := Read(strings.NewReader(in)); err == nil {
			t.Errorf("Read(%q) gave no error", in)
		}
	}
}

func TestReadOtherRate(t *testing.T) {
	const r60 = 60000.0 / 1001
	tl := read(t, otioDoc(t, r60, 216000, []any{
		gap(20, r60),
		clip("/clips/a.MP4", 300, 100, ntsc), // 100 frames at 29.97 = 200 timeline frames
		clip("/clips/b.MP4", 0, 10, r60),
	}))
	a, b := tl.Clips[0], tl.Clips[1]
	if a.RecordStart != 216020 || a.Frames != 200 || b.RecordStart != 216220 {
		t.Fatalf("a = %+v, b = %+v", a, b)
	}
	near(t, "a start", a.Map.Start, 300/ntsc)
	near(t, "rate", tl.Rate, r60)
}

func TestReadFileURL(t *testing.T) {
	c := clip("/x", 0, 10, ntsc)
	c["media_references"].(map[string]any)["DEFAULT_MEDIA"].(map[string]any)["target_url"] = "file:///Volumes/Footage/Day%201%20%26%202/GX010123.MP4"
	tl := read(t, otioDoc(t, ntsc, 0, []any{c}))
	if tl.Clips[0].Path != "/Volumes/Footage/Day 1 & 2/GX010123.MP4" {
		t.Fatalf("path = %q", tl.Clips[0].Path)
	}
}
