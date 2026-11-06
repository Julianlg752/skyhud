// Package design loads SkyHUD HUD designs. A design is a folder with
// design.json (name, pieces and their default places, colors, font) and
// design.setting (the Fusion tools that draw it). The built-in designs ship
// in the same format inside the binary. See docs/designs.md.
package design

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// The pieces a design can have, in drawing order.
var AllPieces = []string{"path", "horizon", "speed", "height", "gforce", "sticks", "battery", "flip"}

// ToolName is the Fusion name of a piece's placing Transform; the design's
// own output tool for the piece is ToolName + "Out", and every tool of the
// piece starts with ToolName.
var ToolName = map[string]string{
	"speed": "Speed", "height": "Height", "gforce": "GForce", "horizon": "Horizon",
	"path": "Path", "sticks": "Sticks", "battery": "Battery", "flip": "Flip",
}

// PieceDef is where a piece sits by default: Fusion coordinates (origin
// bottom left) and a scale.
type PieceDef struct {
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	Size float64 `json:"size,omitempty"`
}

// Design is a loaded design.
type Design struct {
	ID       string                `json:"-"`
	Name     string                `json:"name"`
	Note     string                `json:"note,omitempty"`
	Font     string                `json:"font,omitempty"` // "mono", "sans" or a font name
	Colors   map[string][3]float64 `json:"colors,omitempty"`
	Pieces   map[string]PieceDef   `json:"pieces"`
	Template string                `json:"-"`
	Builtin  bool                  `json:"-"`
}

//go:embed builtin
var builtinFS embed.FS

// Load reads the design in folder dir of fsys.
func Load(fsys fs.FS, dir string) (*Design, error) {
	j, err := fs.ReadFile(fsys, path.Join(dir, "design.json"))
	if err != nil {
		return nil, fmt.Errorf("design %s: %w", path.Base(dir), err)
	}
	d := &Design{}
	if err := json.Unmarshal(j, d); err != nil {
		return nil, fmt.Errorf("design %s: design.json: %w", path.Base(dir), err)
	}
	t, err := fs.ReadFile(fsys, path.Join(dir, "design.setting"))
	if err != nil {
		return nil, fmt.Errorf("design %s: %w", path.Base(dir), err)
	}
	d.ID, d.Template = path.Base(dir), unwrap(string(t))
	if d.Name == "" {
		d.Name = d.ID
	}
	for p, def := range d.Pieces {
		if _, ok := ToolName[p]; !ok {
			return nil, fmt.Errorf("design %s: unknown piece %q", d.ID, p)
		}
		if def.Size == 0 {
			def.Size = 1
			d.Pieces[p] = def
		}
	}
	if d.Colors == nil {
		d.Colors = map[string][3]float64{}
	}
	return d, nil
}

// unwrap returns the tools inside a Fusion clipboard paste
// ({ Tools = ordered() { ... }, ... }), or src as it is.
func unwrap(src string) string {
	i := strings.Index(src, "Tools = ordered()")
	if !strings.HasPrefix(strings.TrimSpace(src), "{") || i < 0 {
		return src
	}
	open := strings.IndexByte(src[i:], '{')
	if open < 0 {
		return src
	}
	close, err := matchBrace(src, i+open)
	if err != nil {
		return src
	}
	return strings.TrimSpace(src[i+open+1:close]) + "\n"
}

// Builtins returns the designs that ship with SkyHUD.
func Builtins() []*Design {
	entries, _ := fs.ReadDir(builtinFS, "builtin")
	var out []*Design
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if d, err := Load(builtinFS, "builtin/"+e.Name()); err == nil {
			d.Builtin = true
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return order(out[i].ID) < order(out[j].ID) })
	return out
}

func order(id string) string {
	for i, b := range []string{"goggles", "race", "fighter", "clean"} {
		if id == b {
			return strconv.Itoa(i)
		}
	}
	return "9" + id
}

// Find returns design id from the user's designs folder, else a built-in.
func Find(id, userDir string) (*Design, error) {
	if userDir != "" && id != "" && !strings.ContainsAny(id, `/\`) && id != "." && id != ".." {
		if _, err := os.Stat(filepath.Join(userDir, id, "design.json")); err == nil {
			return Load(os.DirFS(userDir), id)
		}
	}
	for _, d := range Builtins() {
		if d.ID == id {
			return d, nil
		}
	}
	return nil, fmt.Errorf("design %q not found", id)
}

// List returns the built-in designs followed by the user's.
func List(userDir string) []*Design {
	out := Builtins()
	if userDir == "" {
		return out
	}
	entries, _ := os.ReadDir(userDir)
	for _, e := range entries {
		if e.IsDir() {
			if d, err := Load(os.DirFS(userDir), e.Name()); err == nil {
				out = append(out, d)
			}
		}
	}
	return out
}

// Save writes the design into folder dir (created if missing).
func (d *Design) Save(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	j, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "design.json"), append(j, '\n'), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "design.setting"), []byte(d.Template), 0o644)
}

// Vars fill a design's placeholders.
type Vars struct {
	Mono, Sans string
	Aspect     float64               // frame width / height
	Path       string                // flight path points, Fusion Polyline syntax
	Colors     map[string][3]float64 // overrides of the design's colors
	Font       string                // override of the design's font
	GLabel     string                // g-force label: "G", or "G (EST)" when estimated
}

// Render returns the design's tools with placeholders filled.
func (d *Design) Render(v Vars) (string, error) {
	if v.Aspect <= 0 {
		v.Aspect = 16.0 / 9
	}
	font := d.Font
	if v.Font != "" {
		font = v.Font
	}
	switch font {
	case "", "mono":
		font = v.Mono
	case "sans":
		font = v.Sans
	}
	if v.GLabel == "" {
		v.GLabel = "G"
	}
	repl := []string{"{{mono}}", v.Mono, "{{sans}}", v.Sans, "{{font}}", font,
		"{{aspect}}", num(v.Aspect), "{{path}}", v.Path, "{{glabel}}", v.GLabel}
	colors := map[string][3]float64{}
	for k, c := range d.Colors {
		colors[k] = c
	}
	for k, c := range v.Colors {
		colors[k] = c
	}
	for k, c := range colors {
		repl = append(repl,
			"{{"+k+"}}", fmt.Sprintf("Red = %s, Green = %s, Blue = %s", num(c[0]), num(c[1]), num(c[2])),
			"{{"+k+".r}}", num(c[0]), "{{"+k+".g}}", num(c[1]), "{{"+k+".b}}", num(c[2]))
	}
	out := strings.NewReplacer(repl...).Replace(d.Template)
	if i := strings.Index(out, "{{"); i >= 0 {
		end := min(len(out), i+30)
		return "", errors.New("design " + d.ID + ": unknown placeholder near " + strconv.Quote(out[i:end]))
	}
	return out, nil
}

func num(x float64) string { return strconv.FormatFloat(x, 'g', 10, 64) }
