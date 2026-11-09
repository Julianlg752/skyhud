package main

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Julianlg752/skyhud/internal/design"
	"github.com/Julianlg752/skyhud/internal/dji/djitest"
	"github.com/Julianlg752/skyhud/internal/djilog/djilogtest"
	"github.com/Julianlg752/skyhud/internal/gpmf/gpmftest"
	"github.com/Julianlg752/skyhud/internal/hud"
	"github.com/Julianlg752/skyhud/internal/mp4/mp4test"
	"github.com/Julianlg752/skyhud/internal/secret"
)

const ntsc = 30000.0 / 1001

// otioFile writes a one-track OTIO export whose clips start at media frame 0
// of a file that begins at timecode frame 1000.
func otioFile(t *testing.T, dir string, clips ...map[string]any) string {
	t.Helper()
	rt := func(v float64) map[string]any { return map[string]any{"value": v, "rate": ntsc} }
	var children []any
	for _, c := range clips {
		fx := []any{}
		if s, ok := c["speed"]; ok {
			fx = append(fx, map[string]any{"OTIO_SCHEMA": "LinearTimeWarp.1", "time_scalar": s})
		}
		children = append(children, map[string]any{
			"OTIO_SCHEMA":  "Clip.2",
			"name":         filepath.Base(c["path"].(string)),
			"source_range": map[string]any{"start_time": rt(1000 + c["in"].(float64)), "duration": rt(c["frames"].(float64))},
			"effects":      fx,
			"media_references": map[string]any{"DEFAULT_MEDIA": map[string]any{
				"target_url": c["path"], "available_range": map[string]any{"start_time": rt(1000), "duration": rt(5000)},
			}},
			"active_media_reference_key": "DEFAULT_MEDIA",
		})
	}
	doc := map[string]any{"OTIO_SCHEMA": "Timeline.1", "global_start_time": rt(108000),
		"tracks": map[string]any{"children": []any{map[string]any{"OTIO_SCHEMA": "Track.1", "kind": "Video", "children": children}}}}
	b, _ := json.Marshal(doc)
	p := filepath.Join(dir, "timeline.otio")
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

type shotJob struct {
	ID          string  `json:"id"`
	File        string  `json:"file"`
	Track       int     `json:"track"`
	RecordStart int64   `json:"recordStart"`
	Frames      int     `json:"frames"`
	Speed       float64 `json:"speed"`
	SourceStart float64 `json:"sourceStart"`
	Nudge       float64 `json:"nudge"`
	Out         string  `json:"out"`
}

func writeJobs(t *testing.T, dir, otioPath string, shots ...shotJob) string {
	t.Helper()
	b, _ := json.Marshal(map[string]any{"version": 1, "fps": ntsc, "otio": otioPath, "shots": shots})
	p := filepath.Join(dir, "jobs.json")
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func lines(out string) map[string][]string {
	m := map[string][]string{}
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		f := strings.Split(l, "\t")
		if len(f) >= 3 { // status, id, note[, flips=...]
			m[f[1]] = f
		}
	}
	return m
}

func TestHudWritesCompPerShot(t *testing.T) {
	dir := t.TempDir()
	clip := original(t, dir)
	work := filepath.Join(dir, "work") // created by the command
	jobs := writeJobs(t, dir, "",
		shotJob{ID: "s1", File: clip, Track: 1, RecordStart: 108000, Frames: 30, Speed: 100, Out: filepath.Join(work, "s1.comp")},
		shotJob{ID: "s2", File: clip, Track: 1, RecordStart: 108030, Frames: 20, Speed: 100, Out: filepath.Join(work, "s2.comp")})
	code, out, errs := call("hud", "--jobs", jobs)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errs)
	}
	got := lines(out)
	for _, id := range []string{"s1", "s2"} {
		if got[id] == nil || got[id][0] != "ok" {
			t.Fatalf("%s: %q (all: %q)", id, got[id], out)
		}
		b, err := os.ReadFile(filepath.Join(work, id+".comp"))
		if err != nil || !strings.Contains(string(b), "HUDData = Custom") {
			t.Fatalf("%s comp: %v", id, err)
		}
	}
	if !strings.Contains(got["s1"][2], "timeline export not found") {
		t.Errorf("note = %q", got["s1"][2])
	}
}

func TestHudSkipsFileWithoutTelemetry(t *testing.T) {
	dir := t.TempDir()
	clip := original(t, dir)
	lonely := stabilized(t, t.TempDir(), time.Second)
	jobs := writeJobs(t, dir, "",
		shotJob{ID: "a", File: clip, Track: 1, RecordStart: 0, Frames: 10, Speed: 100, Out: filepath.Join(dir, "a.comp")},
		shotJob{ID: "b", File: lonely, Track: 1, RecordStart: 10, Frames: 10, Speed: 100, Out: filepath.Join(dir, "b.comp")})
	code, out, errs := call("hud", "--jobs", jobs)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errs)
	}
	got := lines(out)
	if got["a"][0] != "ok" || got["b"][0] != "skip" || !strings.Contains(got["b"][2], "GX010123") {
		t.Fatalf("out = %q", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "b.comp")); err == nil {
		t.Fatal("skipped shot still wrote a comp")
	}
}

// firstKey returns the value of the first keyframe of a HUDData input.
func firstKey(t *testing.T, comp []byte, input string) string {
	t.Helper()
	s := string(comp)
	i := strings.Index(s, input+" = BezierSpline")
	if i < 0 {
		t.Fatalf("no %s spline", input)
	}
	s = s[i:]
	s = s[strings.Index(s, "[0] = { ")+len("[0] = { "):]
	return s[:strings.IndexAny(s, ", ")]
}

func TestHudUsesSpeedFromOTIO(t *testing.T) {
	dir := t.TempDir()
	// Second 1 has a GPS fix, second 2 has none.
	clip := mp4test.Build(t, mp4test.Options{Dir: dir, Name: "GX010123.MP4", CreationTime: created,
		Format: "gpmd", Timescale: 1000, SampleDurations: []uint32{1000, 1000},
		Payloads: [][]byte{gpmftest.HERO10Second(3, 40.1), gpmftest.HERO10Second(0, 0)}})
	// The shot starts 1.2 s into the file at half speed; the jobs file alone
	// would say it starts at the file start at full speed.
	tlPath := otioFile(t, dir, map[string]any{"path": clip, "in": 1.2 * ntsc, "frames": 20.0, "speed": 0.5})
	out := filepath.Join(dir, "s.comp")
	jobs := writeJobs(t, dir, tlPath, shotJob{ID: "s", File: clip, Track: 1, RecordStart: 108000, Frames: 20, Speed: 100, Out: out})
	code, stdout, errs := call("hud", "--jobs", jobs)
	if code != 0 || !strings.HasPrefix(stdout, "ok\ts\t") {
		t.Fatalf("exit %d: %s %s", code, stdout, errs)
	}
	if strings.Contains(stdout, "not found") {
		t.Fatalf("OTIO clip not matched: %s", stdout)
	}
	b, _ := os.ReadFile(out)
	if ok := firstKey(t, b, "HUDDataIn2"); ok != "0" {
		t.Fatalf("speed OK on the first frame = %s, want 0 (no GPS lock 1.2 s in)", ok)
	}
}

func TestHudSkipsWhenOTIOClipIsAnotherFile(t *testing.T) {
	dir := t.TempDir()
	clip := original(t, dir)
	tlPath := otioFile(t, dir, map[string]any{"path": "/elsewhere/other.MP4", "in": 0.0, "frames": 20.0})
	jobs := writeJobs(t, dir, tlPath, shotJob{ID: "s", File: clip, Track: 1, RecordStart: 108000, Frames: 20, Speed: 100, Out: filepath.Join(dir, "s.comp")})
	_, out, _ := call("hud", "--jobs", jobs)
	if !strings.HasPrefix(out, "skip\ts\t") || !strings.Contains(out, "other.MP4") {
		t.Fatalf("out = %q", out)
	}
}

func TestHudPathWithSpecialChars(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Wind & Park é")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	clip := original(t, dir)
	jobs := writeJobs(t, dir, "", shotJob{ID: "x", File: clip, Track: 1, Frames: 5, Speed: 100, Out: filepath.Join(dir, "out dir", "x.comp")})
	code, out, errs := call("hud", "--jobs", jobs)
	if code != 0 || !strings.HasPrefix(out, "ok\tx\t") {
		t.Fatalf("exit %d: %q %s", code, out, errs)
	}
}

func TestHudBadJobs(t *testing.T) {
	if code, _, errs := call("hud", "--jobs", filepath.Join(t.TempDir(), "missing.json")); code != 1 || errs == "" {
		t.Fatalf("missing jobs file: exit %d %q", code, errs)
	}
	bad := filepath.Join(t.TempDir(), "bad.json")
	os.WriteFile(bad, []byte("{nope"), 0o644)
	if code, _, errs := call("hud", "--jobs", bad); code != 1 || errs == "" {
		t.Fatalf("bad jobs file: exit %d %q", code, errs)
	}
	if code, _, _ := call("hud"); code != 1 {
		t.Fatal("hud without --jobs should fail")
	}
}

func TestHudSettingsFromJobs(t *testing.T) {
	dir := t.TempDir()
	clip := original(t, dir)
	out := filepath.Join(dir, "s.comp")
	b, _ := json.Marshal(map[string]any{"version": 1, "fps": ntsc,
		"settings": map[string]any{"show": map[string]bool{"gforce": false}, "layoutVersion": 2,
			"layouts": map[string]any{"goggles": map[string]any{"speed": map[string]float64{"x": 0.7, "y": 0.2, "size": 1}}}},
		"shots": []shotJob{{ID: "s", File: clip, Track: 1, Frames: 5, Speed: 100, Out: out}}})
	jobs := filepath.Join(dir, "jobs.json")
	os.WriteFile(jobs, b, 0o644)
	if code, stdout, errs := call("hud", "--jobs", jobs); code != 0 {
		t.Fatalf("exit %d: %s %s", code, stdout, errs)
	}
	c, _ := os.ReadFile(out)
	s := string(c)
	if strings.Contains(s, "GForceOut = TextPlus") || !strings.Contains(s, "HeightOut = TextPlus") {
		t.Fatal("show settings not applied (missing pieces must stay shown)")
	}
	if !strings.Contains(s, "Center = Input { Value = { 0.7, 0.2 }, }") {
		t.Fatal("layout not applied")
	}
}

func TestInstallCommand(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := filepath.Join(t.TempDir(), "Utility")
	code, out, errs := call("install", "--dir", dir)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errs)
	}
	b, err := os.ReadFile(filepath.Join(dir, "SkyHUD.lua"))
	if err != nil || !strings.HasPrefix(string(b), "local SKYHUD_BIN = [==[") || !strings.Contains(out, "Workspace > Scripts > SkyHUD") {
		t.Fatalf("out = %q, err = %v", out, err)
	}
}

// installedBin is where install copies the binary under home.
func installedBin(home string) string {
	return filepath.Join(home, "Library", "Application Support", "SkyHUD", "bin", "skyhud")
}

func binLine(t *testing.T, scripts string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(scripts, "SkyHUD.lua"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b[:bytes.IndexByte(b, '\n')])
}

func TestInstallCopiesBinary(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	scripts := t.TempDir()
	if code, _, errs := call("install", "--dir", scripts); code != 0 {
		t.Fatalf("install: %s", errs)
	}
	self, _ := os.Executable()
	want, _ := os.ReadFile(self)
	got, err := os.ReadFile(installedBin(home))
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("binary not copied: %v", err)
	}
	if fi, _ := os.Stat(installedBin(home)); fi.Mode()&0o111 == 0 {
		t.Errorf("copy not executable: %v", fi.Mode())
	}
	if line := binLine(t, scripts); line != "local SKYHUD_BIN = [==["+installedBin(home)+"]==]" {
		t.Errorf("script points at %q", line)
	}
}

func TestReinstallReplaces(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	scripts := t.TempDir()
	os.MkdirAll(filepath.Dir(installedBin(home)), 0o755)
	os.WriteFile(installedBin(home), []byte("old version"), 0o755)
	os.WriteFile(filepath.Join(scripts, "SkyHUD.lua"), []byte("old script\n"), 0o644)
	if code, _, errs := call("install", "--dir", scripts); code != 0 {
		t.Fatalf("install: %s", errs)
	}
	if b, _ := os.ReadFile(installedBin(home)); string(b) == "old version" {
		t.Error("binary not replaced")
	}
	if line := binLine(t, scripts); !strings.Contains(line, installedBin(home)) {
		t.Errorf("script not replaced: %q", line)
	}
}

func TestInstallHere(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	scripts := t.TempDir()
	if code, _, errs := call("install", "--here", "--dir", scripts); code != 0 {
		t.Fatalf("install: %s", errs)
	}
	if _, err := os.Stat(installedBin(home)); err == nil {
		t.Error("--here copied the binary")
	}
	self, _ := os.Executable()
	if real, err := filepath.EvalSymlinks(self); err == nil {
		self = real
	}
	if line := binLine(t, scripts); line != "local SKYHUD_BIN = [==["+self+"]==]" {
		t.Errorf("script points at %q, want %s", line, self)
	}
}

func avataClip(t *testing.T, dir, name string, created time.Time) string {
	t.Helper()
	var payloads [][]byte
	var durs []uint32
	for i := range 20 {
		payloads = append(payloads, djitest.Packet(djitest.Frame{Quats: [][4]float32{{1, 0, 0, 0}}, Vel: [3]float32{4, 3, 0},
			LatLon: [2]float64{0.7, -1.38}, HeightMM: float32(5000 + 100*i), Device: "DJI Avata2"}))
		durs = append(durs, 100)
	}
	return mp4test.Build(t, mp4test.Options{Dir: dir, Name: name, CreationTime: created,
		Format: "djmd", Timescale: 1000, SampleDurations: durs, Payloads: payloads})
}

func TestInspectAvata(t *testing.T) {
	code, out, errs := call("inspect", "--json", avataClip(t, t.TempDir(), "DJI_20250601083018_0001_D.MP4", created))
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errs)
	}
	var r struct {
		Device   string `json:"device"`
		Channels map[string]struct {
			Estimated bool `json:"estimated"`
		} `json:"channels"`
	}
	json.Unmarshal([]byte(out), &r)
	if r.Device != "DJI Avata2" || !r.Channels["gforce"].Estimated {
		t.Fatalf("inspect = %s", out)
	}
	if _, ok := r.Channels["height"]; !ok {
		t.Fatalf("no height: %s", out)
	}
}

func TestExportStabilizedDJI(t *testing.T) {
	dir := t.TempDir()
	avataClip(t, dir, "DJI_20250601083018_0001_D.MP4", created)
	stab := mp4test.Build(t, mp4test.Options{Dir: dir, Name: "DJI_20250601083018_0001_D_stabilized.mp4",
		CreationTime: created.Add(time.Second), Format: "hvc1", Timescale: 1000,
		SampleDurations: []uint32{500}, Payloads: [][]byte{[]byte("frame")}})
	h := exportHUD(t, stab)
	sp := h.Channels["speed"]
	if len(sp.T) == 0 || sp.T[0] != 0 || len(sp.T) > 6 {
		t.Fatalf("speed times = %v", sp.T)
	}
}

func TestKeyStatusNeverPrintsKey(t *testing.T) {
	store := secret.NewMemory()
	keyStore = store
	t.Cleanup(func() { keyStore = secret.System })
	if code, out, _ := call("key", "status"); code != 0 || !strings.Contains(out, "not stored") {
		t.Fatalf("status before: %d %q", code, out)
	}
	store.Set(secret.DJIKey, "SECRET-123")
	code, out, errs := call("key", "status")
	if code != 0 || !strings.Contains(out, "DJI key: stored") || strings.Contains(out+errs, "SECRET") {
		t.Fatalf("status: %d %q %q", code, out, errs)
	}
}

func TestKeySetReadsStdin(t *testing.T) {
	store := secret.NewMemory()
	keyStore = store
	t.Cleanup(func() { keyStore = secret.System; keyInput = os.Stdin })
	r, w, _ := os.Pipe()
	w.WriteString("  KEY-456  \n")
	w.Close()
	keyInput = r
	code, out, errs := call("key", "set")
	if code != 0 || strings.Contains(out+errs, "KEY-456") {
		t.Fatalf("set: %d %q %q", code, out, errs)
	}
	if v, _ := store.Get(secret.DJIKey); v != "KEY-456" {
		t.Fatalf("stored %q", v)
	}
	r2, w2, _ := os.Pipe()
	w2.Close()
	keyInput = r2
	if code, _, _ := call("key", "set"); code != 1 {
		t.Fatal("empty key accepted")
	}
}

func wave(t float64) float64 { return 10 + 4*math.Sin(t/0.9) + 2*math.Sin(t/0.35) }

// wavyAvata writes a 10 s Avata 2 clip (10 packets per second) whose
// height follows wave.
func wavyAvata(t *testing.T, dir string) string {
	t.Helper()
	var payloads [][]byte
	var durs []uint32
	for i := range 100 {
		payloads = append(payloads, djitest.Packet(djitest.Frame{Quats: [][4]float32{{1, 0, 0, 0}}, Vel: [3]float32{4, 3, 0},
			HeightMM: float32(wave(float64(i)/10) * 1000), Device: "DJI Avata2"}))
		durs = append(durs, 100)
	}
	return mp4test.Build(t, mp4test.Options{Dir: dir, Name: "DJI_20250601083018_0001_D.MP4", CreationTime: created,
		Format: "djmd", Timescale: 1000, SampleDurations: durs, Payloads: payloads})
}

// logsFor writes a matching v12 log starting 30 s before the clip.
func logsFor(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	start := created.Add(-30 * time.Second)
	var frames []djilogtest.Frame
	for i := range 500 {
		at := float64(i) / 10
		frames = append(frames, djilogtest.Frame{At: start.Add(time.Duration(at * float64(time.Second))),
			Height: wave(at - 30), Throttle: uint16(364 + i), Rudder: 1024, Elevator: 1024, Aileron: 364, Battery: 81, Volts: 15.4})
	}
	name := "DJIFlightRecord_" + start.In(time.Local).Format("2006-01-02_[15-04-05]") + ".txt"
	if err := os.WriteFile(filepath.Join(dir, name), djilogtest.V12("DJI Avata 2", start, frames), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func runJobs(t *testing.T, logs string, shots ...shotJob) (string, map[string]string) {
	t.Helper()
	dir := t.TempDir()
	b, _ := json.Marshal(map[string]any{"version": 1, "fps": 10.0, "settings": map[string]any{"logs": logs}, "shots": shots})
	jobs := filepath.Join(dir, "jobs.json")
	os.WriteFile(jobs, b, 0o644)
	code, out, errs := call("hud", "--jobs", jobs)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errs)
	}
	comps := map[string]string{}
	for _, sh := range shots {
		c, _ := os.ReadFile(sh.Out)
		comps[sh.ID] = string(c)
	}
	return out, comps
}

func TestHudAvataWithoutLogs(t *testing.T) {
	dir := t.TempDir()
	out, comps := runJobs(t, "", shotJob{ID: "a", File: wavyAvata(t, dir), Track: 1, Frames: 50, Speed: 100, Out: filepath.Join(dir, "a.comp")})
	if !strings.HasPrefix(out, "ok\ta\t") || strings.Contains(comps["a"], "Sticks = Transform") || !strings.Contains(comps["a"], "G (EST)") {
		t.Fatalf("out %q", out)
	}
}

func TestHudAvataWithLog(t *testing.T) {
	dir := t.TempDir()
	out, comps := runJobs(t, logsFor(t), shotJob{ID: "a", File: wavyAvata(t, dir), Track: 1, Frames: 50, Speed: 100, Out: filepath.Join(dir, "a.comp")})
	if !strings.Contains(out, "flight log DJIFlightRecord_") || !strings.Contains(out, "offset") {
		t.Fatalf("out %q", out)
	}
	for _, want := range []string{"Sticks = Transform", "BatteryOut = TextPlus", "HUDData2 = Custom"} {
		if !strings.Contains(comps["a"], want) {
			t.Errorf("comp lacks %s", want)
		}
	}
}

func TestHudMixedTimeline(t *testing.T) {
	dir := t.TempDir()
	gp := original(t, dir)
	out, comps := runJobs(t, logsFor(t),
		shotJob{ID: "g", File: gp, Track: 1, Frames: 10, Speed: 100, Out: filepath.Join(dir, "g.comp")},
		shotJob{ID: "a", File: wavyAvata(t, dir), Track: 1, RecordStart: 10, Frames: 50, Speed: 100, Out: filepath.Join(dir, "a.comp")})
	got := lines(out)
	if got["g"][0] != "ok" || got["a"][0] != "ok" {
		t.Fatalf("out %q", out)
	}
	if strings.Contains(comps["g"], "Sticks = Transform") || strings.Contains(got["g"][2], "flight log") {
		t.Fatal("GoPro shot got sticks or a flight log")
	}
	if !strings.Contains(comps["a"], "Sticks = Transform") {
		t.Fatal("Avata shot has no sticks")
	}
}

func TestHudNoKeyNote(t *testing.T) {
	keyStore = secret.NewMemory()
	keychainCacheDir = t.TempDir()
	t.Cleanup(func() { keyStore = secret.System; keychainCacheDir = "" })
	logs := t.TempDir()
	start := created.Add(-30 * time.Second)
	name := "DJIFlightRecord_" + start.In(time.Local).Format("2006-01-02_[15-04-05]") + ".txt"
	os.WriteFile(filepath.Join(logs, name), djilogtest.V14Header("DJI Avata 2", start), 0o644)
	dir := t.TempDir()
	out, comps := runJobs(t, logs, shotJob{ID: "a", File: wavyAvata(t, dir), Track: 1, Frames: 50, Speed: 100, Out: filepath.Join(dir, "a.comp")})
	if !strings.HasPrefix(out, "ok\ta\t") || !strings.Contains(out, "no DJI key stored") || !strings.Contains(comps["a"], "HUDData = Custom") {
		t.Fatalf("out %q", out)
	}
}

func runWith(t *testing.T, settings map[string]any, shots ...shotJob) (string, map[string]string) {
	t.Helper()
	dir := t.TempDir()
	b, _ := json.Marshal(map[string]any{"version": 1, "fps": 10.0, "settings": settings, "shots": shots})
	jobs := filepath.Join(dir, "jobs.json")
	os.WriteFile(jobs, b, 0o644)
	code, out, errs := call("hud", "--jobs", jobs)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errs)
	}
	comps := map[string]string{}
	for _, sh := range shots {
		c, _ := os.ReadFile(sh.Out)
		comps[sh.ID] = string(c)
	}
	return out, comps
}

func TestHudUsesChosenDesign(t *testing.T) {
	dir := t.TempDir()
	_, comps := runWith(t, map[string]any{"design": "fighter"}, shotJob{ID: "a", File: wavyAvata(t, dir), Track: 1, Frames: 20, Speed: 100, Out: filepath.Join(dir, "a.comp")})
	if !strings.Contains(comps["a"], "SpeedTick0 = TextPlus") {
		t.Fatal("fighter design not used")
	}
	_, comps = runWith(t, map[string]any{"design": "nope"}, shotJob{ID: "b", File: wavyAvata(t, dir), Track: 1, Frames: 20, Speed: 100, Out: filepath.Join(dir, "b.comp")})
	if !strings.Contains(comps["b"], "SpeedOut = TextPlus") {
		t.Fatal("unknown design should fall back to Goggles")
	}
}

func transformCenter(comp, name string) string {
	i := strings.Index(comp, "\t\t"+name+" = Transform {")
	if i < 0 {
		return ""
	}
	block := comp[i:]
	j := strings.Index(block, "Center = Input { Value = { ")
	return block[j+len("Center = Input { Value = { ") : j+len("Center = Input { Value = { ")+strings.Index(block[j+len("Center = Input { Value = { "):], " }")]
}

func TestOldSettingsIgnored(t *testing.T) {
	dir := t.TempDir()
	clip := wavyAvata(t, dir)
	old := map[string]any{"layout": map[string]any{"speed": map[string]float64{"x": 0.7, "y": 0.2, "size": 0.035}}}
	_, comps := runWith(t, old, shotJob{ID: "a", File: clip, Track: 1, Frames: 20, Speed: 100, Out: filepath.Join(dir, "a.comp")})
	if c := transformCenter(comps["a"], "Speed"); c != "0.11, 0.92" {
		t.Fatalf("old layout used: center %q", c)
	}
	current := map[string]any{"layoutVersion": 2, "design": "goggles", "scale": 1.3,
		"layouts": map[string]any{"goggles": map[string]any{"speed": map[string]float64{"x": 0.7, "y": 0.2, "size": 1.2}}}}
	_, comps = runWith(t, current, shotJob{ID: "b", File: clip, Track: 1, Frames: 20, Speed: 100, Out: filepath.Join(dir, "b.comp")})
	if c := transformCenter(comps["b"], "Speed"); c != "0.7, 0.2" || !strings.Contains(comps["b"], "Size = Input { Value = 1.3, }") {
		t.Fatalf("current layout not used: center %q", c)
	}
}

func TestFlipsReported(t *testing.T) {
	dir := t.TempDir()
	var payloads [][]byte
	var durs []uint32
	for i := range 50 {
		q := [4]float32{1, 0, 0, 0}
		if i >= 20 && i < 26 { // upside down from 2.0 s to 2.6 s
			q = [4]float32{0, 1, 0, 0}
		}
		payloads = append(payloads, djitest.Packet(djitest.Frame{Quats: [][4]float32{q}, Vel: [3]float32{4, 3, 0}, HeightMM: 5000 + float32(i)*10, Device: "DJI Avata2"}))
		durs = append(durs, 100)
	}
	clip := mp4test.Build(t, mp4test.Options{Dir: dir, Name: "DJI_20250601083018_0001_D.MP4", CreationTime: created,
		Format: "djmd", Timescale: 1000, SampleDurations: durs, Payloads: payloads})
	out, _ := runWith(t, nil, shotJob{ID: "a", File: clip, Track: 1, Frames: 40, Speed: 100, SourceStart: 1, Out: filepath.Join(dir, "a.comp")})
	if !strings.Contains(out, "\tflips=1\n") {
		t.Fatalf("out = %q", out)
	}
}

func TestDesignCheckCommand(t *testing.T) {
	dir := t.TempDir()
	if code, out, errs := call("design", "save", "--from", "race", "--name", "My Race", "--dir", dir, "--color", "accent=#00ff00", "--font", "mono"); code != 0 || !strings.Contains(out, "my-race") {
		t.Fatalf("save: %d %q %q", code, out, errs)
	}
	folder := filepath.Join(dir, "my-race")
	j, _ := os.ReadFile(filepath.Join(folder, "design.json"))
	if !strings.Contains(string(j), `"My Race"`) || !strings.Contains(string(j), `"mono"`) || !strings.Contains(string(j), `"Based on Race"`) {
		t.Fatalf("design.json = %s", j)
	}
	if code, out, _ := call("design", "check", folder); code != 0 || !strings.Contains(out, "ok") {
		t.Fatalf("check good: %d %q", code, out)
	}
	bad := filepath.Join(dir, "bad")
	os.MkdirAll(bad, 0o755)
	os.WriteFile(filepath.Join(bad, "design.json"), []byte(`{"name": "Bad", "pieces": {"speed": {"x": 0.5, "y": 0.5}}}`), 0o644)
	os.WriteFile(filepath.Join(bad, "design.setting"), []byte("SpeedOut = TextPlus {\n\tInputs = {\n\t\tStyledText = Input { Expression = \"io.open(\\\"x\\\")\", },\n\t},\n},\n"), 0o644)
	if code, out, _ := call("design", "check", bad); code != 1 || !strings.Contains(out, `"io"`) {
		t.Fatalf("check bad: %d %q", code, out)
	}
}

func TestDesignListJSON(t *testing.T) {
	dir := t.TempDir()
	call("design", "save", "--from", "clean", "--name", "Quiet", "--dir", dir)
	code, out, _ := call("design", "list", "--json", "--dir", dir)
	var list []struct {
		ID, Name string
		Builtin  bool
		Pieces   []string
		Problems []string
	}
	if code != 0 || json.Unmarshal([]byte(out), &list) != nil {
		t.Fatalf("list: %d %q", code, out)
	}
	ids := map[string]bool{}
	for _, d := range list {
		ids[d.ID] = true
	}
	if !ids["goggles"] || !ids["fighter"] || !ids["quiet"] || len(list) != 5 {
		t.Fatalf("list = %+v", list)
	}
}

func TestDesignPreview(t *testing.T) {
	out := filepath.Join(t.TempDir(), "p.comp")
	if code, _, errs := call("design", "preview", "race", "-o", out); code != 0 {
		t.Fatalf("preview: %s", errs)
	}
	b, _ := os.ReadFile(out)
	for _, want := range []string{"SpeedOut", "SticksOut", "BatteryOut", "PathOut", "FlipOut", "FXOut", "HUD = Transform"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("preview lacks %s", want)
		}
	}
}

func TestDesignListTSV(t *testing.T) {
	code, out, _ := call("design", "list", "--tsv")
	first := strings.Split(strings.Split(out, "\n")[0], "\t")
	if code != 0 || len(first) != 7 || first[0] != "goggles" || first[1] != "Goggles" || first[2] != "builtin" || !strings.Contains(first[3], "horizon") || first[4] != "0" {
		t.Fatalf("tsv = %q", out)
	}
}

// A saved design never takes a built-in design's id, which would hide it.
func TestDesignSaveKeepsBuiltinNames(t *testing.T) {
	dir := t.TempDir()
	code, out, errs := call("design", "save", "--from", "race", "--name", "Race", "--dir", dir, "--color", "accent=#00ff00")
	if code != 0 || strings.TrimSpace(out) != "race-2" {
		t.Fatalf("save: %d %q %q", code, out, errs)
	}
	if code, out, _ := call("design", "list", "--tsv", "--dir", dir); code != 0 || strings.Count(out, "race\t") != 1 || !strings.Contains(out, "race-2\t") {
		t.Fatalf("list: %q", out)
	}
}

// Colors and font picked in the panel apply to that design only.
func TestHudUsesDesignColors(t *testing.T) {
	dir := t.TempDir()
	settings := map[string]any{"design": "fighter",
		"designColors": map[string]any{"fighter": map[string]any{"main": []float64{1, 0.25, 0}}, "race": map[string]any{"main": []float64{0, 0, 1}}},
		"designFonts":  map[string]any{"fighter": "sans", "race": "mono"}}
	_, comps := runWith(t, settings, shotJob{ID: "a", File: wavyAvata(t, dir), Track: 1, Frames: 20, Speed: 100, Out: filepath.Join(dir, "a.comp")})
	if !strings.Contains(comps["a"], "Green1 = Input { Value = 0.25, }") || strings.Contains(comps["a"], "Value = 0.365") {
		t.Error("fighter colors not applied")
	}
	if !strings.Contains(comps["a"], `Font = Input { Value = "`+hud.DefaultSans(runtime.GOOS)+`", }`) {
		t.Error("fighter font not applied")
	}
}

func TestDesignListTSVColors(t *testing.T) {
	code, out, _ := call("design", "list", "--tsv")
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		f := strings.Split(line, "\t")
		if f[0] == "race" {
			if code != 0 || len(f) != 7 || !strings.HasPrefix(f[5], "main=#ffffff,accent=#") || !strings.Contains(f[5], ",accent2=#") || f[6] != "sans" {
				t.Fatalf("race line %q", line)
			}
			return
		}
	}
	t.Fatalf("no race line: %q", out)
}

func TestHudImperialUnits(t *testing.T) {
	dir := t.TempDir()
	out, comps := runWith(t, map[string]any{"design": "race", "units": "imperial"}, shotJob{ID: "a", File: wavyAvata(t, dir), Track: 1, Frames: 20, Speed: 100, Out: filepath.Join(dir, "a.comp")})
	if !strings.Contains(comps["a"], `Text(\"MPH\")`) || strings.Contains(out, "units") {
		t.Fatalf("imperial units not applied: %q", out)
	}

	// A design saved before the units option keeps its fixed labels, so it
	// stays metric and the panel says why.
	designs := t.TempDir()
	old, err := design.Find("clean", "")
	if err != nil {
		t.Fatal(err)
	}
	old.ID, old.Name = "old", "Old"
	old.Template = strings.NewReplacer("{{speedunit}}", "km/h", "{{heightunit}}", "m", " * {{speedfactor}}", "", " * {{heightfactor}}", "").Replace(old.Template)
	if err := old.Save(filepath.Join(designs, "old")); err != nil {
		t.Fatal(err)
	}
	out, comps = runWith(t, map[string]any{"design": "old", "designs": designs, "units": "imperial"}, shotJob{ID: "b", File: wavyAvata(t, dir), Track: 1, Frames: 20, Speed: 100, Out: filepath.Join(dir, "b.comp")})
	if !strings.Contains(comps["b"], "km/h") || !strings.Contains(out, "design Old shows fixed km/h and m labels") {
		t.Fatalf("old design: %q", out)
	}

	// A design that only shows speed, labelled with the placeholders, follows the units with no note.
	speedOnly, _ := design.Find("clean", "")
	speedOnly.ID, speedOnly.Name = "speedonly", "Speed only"
	speedOnly.Template = strings.NewReplacer("{{heightunit}}", "m", "{{heightfactor}}", "1").Replace(speedOnly.Template)
	if err := speedOnly.Save(filepath.Join(designs, "speedonly")); err != nil {
		t.Fatal(err)
	}
	out, comps = runWith(t, map[string]any{"design": "speedonly", "designs": designs, "units": "imperial"}, shotJob{ID: "c", File: wavyAvata(t, dir), Track: 1, Frames: 20, Speed: 100, Out: filepath.Join(dir, "c.comp")})
	if !strings.Contains(comps["c"], "mph") || strings.Contains(out, "fixed km/h") {
		t.Fatalf("speed-only design: %q", out)
	}
}
