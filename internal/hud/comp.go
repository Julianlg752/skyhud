package hud

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/Julianlg752/skyhud/internal/design"
)

// Place is where a piece sits: the Center of its placing Transform (0..1,
// origin bottom left) and its Size (scale, 1 = as designed).
type Place struct {
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	Size float64 `json:"size"`
}

// Settings are the user's HUD choices for one design.
type Settings struct {
	Show   map[Piece]bool  `json:"show"`
	Height HeightMode      `json:"height"`
	Layout map[Piece]Place `json:"layout"` // overrides of the design's places
	Scale  float64         `json:"scale"`  // whole HUD, 1 = as designed
}

// DefaultSettings shows every piece at the design's places.
func DefaultSettings() Settings {
	return Settings{
		Show:   map[Piece]bool{Speed: true, Height: true, GForce: true, Horizon: true, Sticks: true, Battery: true, Path: true, Flip: true},
		Height: FromStart,
		Layout: map[Piece]Place{},
		Scale:  1,
	}
}

// DefaultFont picks a monospace font that ships with the system.
func DefaultFont(goos string) string {
	switch goos {
	case "darwin":
		return "Menlo"
	case "windows":
		return "Consolas"
	}
	return "DejaVu Sans Mono"
}

// DefaultSans picks a sans-serif font that ships with the system.
func DefaultSans(goos string) string {
	switch goos {
	case "darwin":
		return "Helvetica Neue"
	case "windows":
		return "Segoe UI"
	}
	return "DejaVu Sans"
}

// Style is how the comp looks beyond the user's settings.
type Style struct {
	Mono, Sans      string
	GForceEstimated bool    // label g-force "G (EST)"
	Aspect          float64 // frame width / height; 0 means 16:9
	Path            string  // the recording's flight path, Fusion Polyline points
	Colors          map[string][3]float64
	Font            string
}

// slot is one keyframed input of a data tool.
type slot struct {
	in    int
	value Piece
	ok    bool // keyframe the OK flags of value instead of its values
	step  float64
}

// The data tools (Fusion's Custom tool has eight number inputs). Designs
// read these; docs/designs.md lists them.
var dataTools = []struct {
	name  string
	slots []slot
}{
	{"HUDData", []slot{
		{1, Speed, false, 0.5}, {2, Speed, true, 0}, {3, Height, false, 0.1}, {4, Height, true, 0},
		{5, GForce, false, 0.02}, {6, GForce, true, 0}, {7, Horizon, false, 0.5}, {8, Horizon, true, 0},
	}},
	{"HUDData2", []slot{
		{1, StickThrottle, false, 0.01}, {2, StickYaw, false, 0.01}, {3, StickPitch, false, 0.01}, {4, StickRoll, false, 0.01},
		{5, Sticks, true, 0}, {6, Battery, false, 0.5}, {7, BatteryVolts, false, 0.02}, {8, Battery, true, 0},
	}},
	{"HUDData3", []slot{
		{1, Pitch, false, 0.5}, {2, Pitch, true, 0}, {3, PathX, false, 0.002}, {4, PathY, false, 0.002},
		{5, PathProgress, false, 0.001}, {6, Path, true, 0}, {7, Flip, false, 0.5}, {8, FlipAge, false, 0.5},
	}},
}

// has reports whether a piece has data on any frame.
func has(v Values, p Piece) bool {
	for _, ok := range v[p].OK {
		if ok {
			return true
		}
	}
	return false
}

// WriteComp writes the HUD for one shot as a Fusion composition: the data
// tools keyframed with the values, the design's tools for every shown piece
// that has data, a Transform per piece that places it, and a HUD Transform
// that scales the whole HUD.
func WriteComp(w io.Writer, v Values, d *design.Design, s Settings, st Style) error {
	if st.Aspect <= 0 {
		st.Aspect = 16.0 / 9
	}
	if s.Scale <= 0 {
		s.Scale = 1
	}
	label := "G"
	if st.GForceEstimated {
		label = "G (EST)"
	}
	vars := design.Vars{Mono: st.Mono, Sans: st.Sans, Aspect: st.Aspect, Path: st.Path, Colors: st.Colors, Font: st.Font, GLabel: label}
	if ps := design.CheckWith(d, vars); len(ps) > 0 {
		return fmt.Errorf("design %s was refused: %s", d.Name, ps[0])
	}
	src, err := d.Render(vars)
	if err != nil {
		return err
	}
	tools, err := design.ParseTools(src)
	if err != nil {
		return fmt.Errorf("design %s: %w", d.ID, err)
	}
	shown := map[string]bool{}
	for p := range d.Pieces {
		if s.Show[Piece(p)] && has(v, Piece(p)) {
			shown[p] = true
		}
	}
	n := 0
	for _, p := range Pieces {
		n = max(n, len(v[p].V))
	}
	last := max(n-1, 0)
	b := bufio.NewWriter(w)
	fmt.Fprintf(b, "Composition {\n\tCurrentTime = 0,\n\tRenderRange = { 0, %d },\n\tGlobalRange = { 0, %d },\n\tHiQ = true,\n\tTools = ordered() {\n", last, last)
	for _, dt := range dataTools {
		writeData(b, dt.name, dt.slots, v)
	}
	fx := false
	for _, t := range tools {
		p := design.PieceOf(t.Name)
		if p == "" && strings.HasPrefix(t.Name, "FX") || shown[p] {
			fmt.Fprintf(b, "\t\t%s = %s {%s},\n", t.Name, t.Type, t.Body)
			fx = fx || t.Name == "FXOut"
		}
	}
	var layers []string
	if fx {
		layers = append(layers, "FXOut")
	}
	for _, p := range design.AllPieces {
		if !shown[p] {
			continue
		}
		at, ok := s.Layout[Piece(p)]
		if !ok {
			def := d.Pieces[p]
			at = Place{X: def.X, Y: def.Y, Size: def.Size}
		}
		if at.Size <= 0 {
			at.Size = 1
		}
		name := design.ToolName[p]
		fmt.Fprintf(b, "\t\t%s = Transform {\n\t\t\tInputs = {\n\t\t\t\tCenter = Input { Value = { %s, %s }, },\n\t\t\t\tSize = Input { Value = %s, },\n\t\t\t\tInput = Input { SourceOp = %q, Source = \"Output\", },\n\t\t\t},\n\t\t},\n",
			name, num(at.X), num(at.Y), num(at.Size), name+"Out")
		layers = append(layers, name)
	}
	out := ""
	for i, l := range layers {
		if i == 0 {
			out = l
			continue
		}
		m := fmt.Sprintf("SkyHUDMerge%d", i)
		writeMerge(b, m, out, l)
		out = m
	}
	if out != "" {
		fmt.Fprintf(b, "\t\tHUD = Transform {\n\t\t\tInputs = {\n\t\t\t\tSize = Input { Value = %s, },\n\t\t\t\tInput = Input { SourceOp = %q, Source = \"Output\", },\n\t\t\t},\n\t\t},\n", num(s.Scale), out)
	}
	b.WriteString("\t\tMediaOut1 = Saver {\n\t\t\tInputs = {\n\t\t\t\tIndex = Input { Value = \"0\", },\n")
	if out != "" {
		b.WriteString("\t\t\t\tInput = Input { SourceOp = \"HUD\", Source = \"Output\", },\n")
	}
	b.WriteString("\t\t\t},\n\t\t},\n\t},\n}\n")
	return b.Flush()
}

// PathPolyline renders path points (from PathPoints) as Fusion Polyline
// points, keeping the shape round on a frame of the given aspect.
func PathPolyline(pts [][2]float64, aspect float64) string {
	if aspect <= 0 {
		aspect = 16.0 / 9
	}
	var parts []string
	for _, p := range pts {
		parts = append(parts, fmt.Sprintf("{ Linear = true, X = %s, Y = %s }", num(p[0]), num(p[1]*aspect)))
	}
	if len(parts) == 0 {
		return "{ Linear = true, X = 0, Y = 0 }"
	}
	return strings.Join(parts, ", ")
}

func writeData(b *bufio.Writer, name string, slots []slot, v Values) {
	fmt.Fprintf(b, "\t\t%s = Custom {\n\t\t\tNameSet = true,\n\t\t\tInputs = {\n", name)
	for _, sl := range slots {
		fmt.Fprintf(b, "\t\t\t\tNumberIn%d = Input { SourceOp = \"%sIn%d\", Source = \"Value\", },\n", sl.in, name, sl.in)
	}
	b.WriteString("\t\t\t},\n\t\t},\n")
	n := 0
	for _, sl := range slots {
		n = max(n, len(v[sl.value].V), len(v[sl.value].OK))
	}
	for _, sl := range slots {
		s := v[sl.value]
		spline := fmt.Sprintf("%sIn%d", name, sl.in)
		if len(s.V) == 0 && len(s.OK) == 0 { // no data at all: a flat zero
			s = Series{V: make([]float64, max(n, 1)), OK: make([]bool, max(n, 1))}
		}
		if sl.ok {
			writeSpline(b, spline, flags(s.OK), thinFlags(s.OK))
		} else {
			writeSpline(b, spline, s.V, thin(s.V, sl.step))
		}
	}
}

func writeMerge(b *bufio.Writer, name, bg, fg string) {
	fmt.Fprintf(b, "\t\t%s = Merge {\n\t\t\tInputs = {\n\t\t\t\tBackground = Input { SourceOp = %q, Source = \"Output\", },\n\t\t\t\tForeground = Input { SourceOp = %q, Source = \"Output\", },\n\t\t\t\tPerformDepthMerge = Input { Value = 0, },\n\t\t\t},\n\t\t},\n", name, bg, fg)
}

// writeSpline writes a linear BezierSpline with keys at the given frames.
func writeSpline(b *bufio.Writer, name string, v []float64, keys []int) {
	fmt.Fprintf(b, "\t\t%s = BezierSpline {\n\t\t\tSplineColor = { Red = 233, Green = 19, Blue = 5 },\n\t\t\tNameSet = true,\n\t\t\tKeyFrames = {\n", name)
	for i, f := range keys {
		fmt.Fprintf(b, "\t\t\t\t[%d] = { %s", f, num(v[f]))
		if i > 0 {
			p := keys[i-1]
			fmt.Fprintf(b, ", LH = { %s, %s }", num(float64(f)-float64(f-p)/3), num(v[f]-(v[f]-v[p])/3))
		}
		if i+1 < len(keys) {
			q := keys[i+1]
			fmt.Fprintf(b, ", RH = { %s, %s }", num(float64(f)+float64(q-f)/3), num(v[f]+(v[q]-v[f])/3))
		}
		b.WriteString(", Flags = { Linear = true } },\n")
	}
	b.WriteString("\t\t\t},\n\t\t},\n")
}

// thin keeps the first and last frame and every frame whose value moved more
// than step from the last kept key, plus the frame before it, so a flat
// stretch stays flat up to a jump instead of sliding toward it.
func thin(v []float64, step float64) []int {
	if len(v) == 0 {
		return nil
	}
	keys := []int{0}
	for i := 1; i < len(v)-1; i++ {
		if math.Abs(v[i]-v[keys[len(keys)-1]]) > step {
			if i-1 > keys[len(keys)-1] {
				keys = append(keys, i-1)
			}
			keys = append(keys, i)
		}
	}
	if len(v) > 1 {
		keys = append(keys, len(v)-1)
	}
	return keys
}

// thinFlags keeps the first and last frame and both sides of every change, so
// linear interpolation never blends a known value into a hole.
func thinFlags(ok []bool) []int {
	if len(ok) == 0 {
		return nil
	}
	keep := map[int]bool{0: true, len(ok) - 1: true}
	for i := 1; i < len(ok); i++ {
		if ok[i] != ok[i-1] {
			keep[i-1], keep[i] = true, true
		}
	}
	var keys []int
	for i := range ok {
		if keep[i] {
			keys = append(keys, i)
		}
	}
	return keys
}

func flags(ok []bool) []float64 {
	v := make([]float64, len(ok))
	for i, b := range ok {
		if b {
			v[i] = 1
		}
	}
	return v
}

func num(x float64) string {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		x = 0
	}
	return strconv.FormatFloat(x, 'g', 10, 64)
}

// luaString quotes s for a Fusion setting file (Lua syntax).
func luaString(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)
	return `"` + r.Replace(s) + `"`
}
