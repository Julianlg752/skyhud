package hud

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"github.com/Julianlg752/skyhud/internal/design"
)

// steady returns n frames of fixed, known values for every piece.
func steady(n int, speed, height, g, angle float64) Values {
	v := Values{}
	set := func(p Piece, x float64) {
		s := Series{V: make([]float64, n), OK: make([]bool, n)}
		for i := range n {
			s.V[i], s.OK[i] = x, true
		}
		v[p] = s
	}
	set(Speed, speed)
	set(Height, height)
	set(GForce, g)
	set(Horizon, angle)
	return v
}

// withLogAndPath adds sticks, battery, path and flip data.
func withLogAndPath(v Values, n int) Values {
	for _, p := range []Piece{Sticks, StickThrottle, StickYaw, StickPitch, StickRoll, Battery, BatteryVolts, Path, PathX, PathY, PathProgress, Flip, FlipAge, Pitch} {
		s := Series{V: make([]float64, n), OK: make([]bool, n)}
		for i := range n {
			s.V[i], s.OK[i] = 0.25, true
		}
		v[p] = s
	}
	return v
}

func goggles(t *testing.T) *design.Design {
	t.Helper()
	d, err := design.Find("goggles", "")
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func comp(t *testing.T, v Values, d *design.Design, s Settings) string {
	t.Helper()
	var b bytes.Buffer
	if err := WriteComp(&b, v, d, s, Style{Mono: "Menlo", Sans: "Helvetica", Aspect: 4.0 / 3}); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func defined(out string) map[string]bool {
	m := map[string]bool{}
	for _, x := range regexp.MustCompile(`(?m)^\t\t(\w+) = \w+ \{`).FindAllStringSubmatch(out, -1) {
		m[x[1]] = true
	}
	return m
}

func TestCompUsesDesignTools(t *testing.T) {
	out := comp(t, withLogAndPath(steady(100, 42, 12, 1.2, 30), 100), goggles(t), DefaultSettings())
	tools := defined(out)
	for _, want := range []string{"HUDData", "HUDData2", "HUDData3", "SpeedOut", "Speed", "SticksOut", "Sticks", "PathOut", "Path", "FlipOut", "Flip", "HUD", "MediaOut1"} {
		if !tools[want] {
			t.Errorf("comp lacks tool %s", want)
		}
	}
	if strings.Contains(out, "{{") {
		t.Fatal("placeholder left in the comp")
	}
	if !strings.Contains(out, "GlobalRange = { 0, 99 }") || !strings.Contains(out, `Value = "Menlo"`) {
		t.Fatal("range or font missing")
	}
	if strings.Count(out, "{") != strings.Count(out, "}") {
		t.Fatal("unbalanced braces")
	}
	for _, m := range regexp.MustCompile(`SourceOp = "(\w+)"`).FindAllStringSubmatch(out, -1) {
		if !tools[m[1]] {
			t.Errorf("SourceOp %q is not defined", m[1])
		}
	}
}

func TestPieceTransformsPlaced(t *testing.T) {
	s := DefaultSettings()
	s.Layout = map[Piece]Place{Height: {X: 0.8, Y: 0.25, Size: 1.5}}
	out := comp(t, steady(10, 1, 2, 3, 4), goggles(t), s)
	block := out[strings.Index(out, "\t\tHeight = Transform {"):]
	block = block[:strings.Index(block, "\n\t\t},")]
	if !strings.Contains(block, "Center = Input { Value = { 0.8, 0.25 }, }") || !strings.Contains(block, "Size = Input { Value = 1.5, }") || !strings.Contains(block, `SourceOp = "HeightOut"`) {
		t.Fatalf("height transform:\n%s", block)
	}
	speed := out[strings.Index(out, "\t\tSpeed = Transform {"):]
	if !strings.Contains(speed[:300], "Center = Input { Value = { 0.11, 0.92 }, }") {
		t.Fatalf("speed not at the design's default place:\n%s", speed[:300])
	}
}

func TestHiddenAndDatalessPiecesLeftOut(t *testing.T) {
	s := DefaultSettings()
	s.Show[GForce] = false
	out := comp(t, steady(10, 1, 2, 3, 4), goggles(t), s) // no sticks, battery, path or flip data
	tools := defined(out)
	for _, gone := range []string{"GForceOut", "GForce", "SticksOut", "SticksDotL", "BatteryOut", "PathOut", "PathLine", "FlipOut"} {
		if tools[gone] {
			t.Errorf("%s should be left out", gone)
		}
	}
	if !tools["SpeedOut"] || !tools["HorizonOut"] {
		t.Fatal("pieces with data missing")
	}
}

func TestHUDScale(t *testing.T) {
	s := DefaultSettings()
	s.Scale = 1.4
	out := comp(t, steady(10, 1, 2, 3, 4), goggles(t), s)
	hud := out[strings.Index(out, "\t\tHUD = Transform {"):]
	if !strings.Contains(hud[:min(len(hud), 400)], "Size = Input { Value = 1.4, }") {
		t.Fatalf("HUD transform:\n%s", hud[:min(len(hud), 400)])
	}
}

func TestCompPassesCheck(t *testing.T) {
	for _, d := range design.Builtins() {
		if ps := design.Check(d); len(ps) != 0 {
			t.Errorf("%s: %v", d.ID, ps)
		}
		out := comp(t, withLogAndPath(steady(20, 42, 12, 1.2, 30), 20), d, DefaultSettings())
		if !strings.Contains(out, "MediaOut1 = Saver") {
			t.Errorf("%s: no output", d.ID)
		}
	}
}

func TestGForceEstimatedLabel(t *testing.T) {
	var b bytes.Buffer
	WriteComp(&b, steady(10, 1, 2, 3, 4), goggles(t), DefaultSettings(), Style{Mono: "Menlo", GForceEstimated: true})
	if !strings.Contains(b.String(), `G (EST)\\n%.1f`) {
		t.Fatal("estimated g-force not labeled")
	}
	if strings.Contains(comp(t, steady(10, 1, 2, 3, 4), goggles(t), DefaultSettings()), "G (EST)") {
		t.Fatal("measured g-force labeled as estimated")
	}
}

func TestDefaultSettings(t *testing.T) {
	s := DefaultSettings()
	if s.Height != FromStart || s.Scale != 1 || len(s.Show) != 8 {
		t.Fatalf("defaults = %+v", s)
	}
}

func TestCompNoLocation(t *testing.T) {
	out := strings.ToLower(comp(t, withLogAndPath(steady(5, 1, 2, 3, 4), 5), goggles(t), DefaultSettings()))
	for _, bad := range []string{"gps", "lat", "lon"} {
		if regexp.MustCompile(`\b` + bad + `\b`).MatchString(out) {
			t.Errorf("comp mentions %q", bad)
		}
	}
}

func TestDefaultFont(t *testing.T) {
	for goos, want := range map[string]string{"darwin": "Menlo", "windows": "Consolas", "linux": "DejaVu Sans Mono"} {
		if got := DefaultFont(goos); got != want {
			t.Errorf("DefaultFont(%s) = %s, want %s", goos, got, want)
		}
	}
}

func TestThinning(t *testing.T) {
	if keys := thin([]float64{5, 5, 5, 5, 5, 5, 5, 5, 5, 5}, 0.5); len(keys) != 2 || keys[0] != 0 || keys[1] != 9 {
		t.Fatalf("constant: keys = %v", keys)
	}
	got := thinFlags([]bool{true, true, true, false, false, true})
	want := []int{0, 2, 3, 4, 5}
	if len(got) != len(want) {
		t.Fatalf("flags: keys = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("flags: keys = %v, want %v", got, want)
		}
	}
	// A slow drift is kept whenever it moves past the threshold.
	drift := make([]float64, 100)
	for i := range drift {
		drift[i] = float64(i) * 0.1
	}
	if keys := thin(drift, 0.5); len(keys) < 15 || len(keys) > 40 { // pairs of keys at each step, still far under 100
		t.Fatalf("drift: %d keys", len(keys))
	}
}

// shown evaluates linear keys at frame f, as Fusion does between keyframes.
func shown(v []float64, keys []int, f int) float64 {
	for i := 0; i+1 < len(keys); i++ {
		a, b := keys[i], keys[i+1]
		if f >= a && f <= b {
			return v[a] + (v[b]-v[a])*float64(f-a)/float64(b-a)
		}
	}
	return v[keys[len(keys)-1]]
}

func TestThinningKeepsFlatBeforeJump(t *testing.T) {
	v := make([]float64, 200)
	for i := range 100 {
		v[i] = 80 // then 0 from frame 100, as when GPS is lost
	}
	keys := thin(v, 0.5)
	for _, f := range []int{1, 50, 99} {
		if got := shown(v, keys, f); got != 80 {
			t.Errorf("frame %d shows %v, want 80 (keys %v)", f, got, keys)
		}
	}
	// A slow ramp stays within the step everywhere.
	for i := range v {
		v[i] = float64(i) * 0.07
	}
	keys = thin(v, 0.5)
	for f := range v {
		if d := shown(v, keys, f) - v[f]; d > 0.5 || d < -0.5 {
			t.Fatalf("frame %d off by %v", f, d)
		}
	}
}

// A design that fails the check is never written into a comp, even if the
// caller forgot to check it.
func TestCompRefusesUncheckedDesign(t *testing.T) {
	d := *goggles(t)
	d.Template = strings.Replace(d.Template, `Size = Input { Value = 0.035, },`, `Size = Input { Expression = "os.execute(\"touch x\")", },`, 1)
	if d.Template == goggles(t).Template {
		t.Fatal("test setup: replacement did not apply")
	}
	var b bytes.Buffer
	err := WriteComp(&b, steady(10, 1, 2, 3, 4), &d, DefaultSettings(), Style{Mono: "Menlo"})
	if err == nil || strings.Contains(b.String(), "execute") {
		t.Fatalf("err %v, comp has payload: %v", err, strings.Contains(b.String(), "execute"))
	}
}

func TestImperialUnits(t *testing.T) {
	s := DefaultSettings()
	s.Units = Imperial
	out := comp(t, steady(10, 100, 10, 1, 0), goggles(t), s)
	// the data stays metric (gauges keep their scale); the labels convert it
	for _, want := range []string{"[0] = { 100", "[0] = { 10", "mph", "ft", "HUDData.NumberIn1 * 0.6213711922", "HUDData.NumberIn3 * 3.280839895"} {
		if !strings.Contains(out, want) {
			t.Errorf("imperial comp lacks %q", want)
		}
	}
	metric := comp(t, steady(10, 100, 10, 1, 0), goggles(t), DefaultSettings())
	for _, want := range []string{"[0] = { 100", "km/h", "HUDData.NumberIn1 * 1"} {
		if !strings.Contains(metric, want) {
			t.Errorf("metric comp lacks %q", want)
		}
	}
}

func TestFighterTapesInFeet(t *testing.T) {
	d, err := design.Find("fighter", "")
	if err != nil {
		t.Fatal(err)
	}
	s := DefaultSettings()
	s.Units = Imperial
	out := comp(t, withLogAndPath(steady(10, 100, 10, 1, 0), 10), d, s)
	// tick 8 of the height tape reads 120 ft and sits at 120 ft (36.6 m), so the tape covers the same height as in meters
	if !strings.Contains(out, `string.format(\"%.0f\", 8 * 15)`) || !strings.Contains(out, "(8 * 15 / 3.280839895 - HUDData.NumberIn3)") {
		t.Error("height tape ticks not in feet")
	}
}
