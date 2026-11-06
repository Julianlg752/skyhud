package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/Julianlg752/skyhud/internal/design"
	"github.com/Julianlg752/skyhud/internal/hud"
)

// designCommand lists, checks, saves and previews HUD designs.
//
//	skyhud design list [--dir D] [--json]
//	skyhud design check <folder>
//	skyhud design save --from ID --name NAME --dir D [--color key=#rrggbb]... [--font F] [--layout JSON]
//	skyhud design preview <folder|id> -o out.comp [--dir D] [--seconds S]
func designCommand(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: skyhud design list|check|save|preview")
	}
	switch args[0] {
	case "list":
		return designList(args[1:], stdout, stderr)
	case "check":
		return designCheck(args[1:], stdout)
	case "save":
		return designSave(args[1:], stdout, stderr)
	case "preview":
		return designPreview(args[1:], stderr)
	}
	return fmt.Errorf("unknown design command %q", args[0])
}

type designInfo struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Note     string   `json:"note"`
	Builtin  bool     `json:"builtin"`
	Pieces   []string `json:"pieces"`
	Colors   []string `json:"colors"` // name=#rrggbb, main first
	Font     string   `json:"font"`
	Problems []string `json:"problems"`
}

func designList(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("design list", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("dir", "", "the user's designs folder")
	asJSON := fs.Bool("json", false, "print JSON")
	asTSV := fs.Bool("tsv", false, "print id, name, builtin|user, pieces, problem count, tab-separated (for the panel)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	var list []designInfo
	for _, d := range design.List(*dir) {
		info := designInfo{ID: d.ID, Name: d.Name, Note: d.Note, Builtin: d.Builtin, Pieces: []string{}, Colors: colorList(d.Colors), Font: d.Font, Problems: []string{}}
		if info.Font == "" {
			info.Font = "mono"
		}
		for _, p := range design.AllPieces {
			if _, ok := d.Pieces[p]; ok {
				info.Pieces = append(info.Pieces, p)
			}
		}
		for _, p := range design.Check(d) {
			info.Problems = append(info.Problems, p.String())
		}
		list = append(list, info)
	}
	if *asJSON {
		return json.NewEncoder(stdout).Encode(list)
	}
	if *asTSV {
		for _, d := range list {
			kind := "user"
			if d.Builtin {
				kind = "builtin"
			}
			fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\t%d\t%s\t%s\n", field(d.ID), field(d.Name), kind, strings.Join(d.Pieces, ","), len(d.Problems),
				field(strings.Join(d.Colors, ",")), field(d.Font))
		}
		return nil
	}
	for _, d := range list {
		kind := "yours"
		if d.Builtin {
			kind = "built in"
		}
		status := "ok"
		if len(d.Problems) > 0 {
			status = fmt.Sprintf("%d problem(s)", len(d.Problems))
		}
		fmt.Fprintf(stdout, "%-12s %-16s %-9s %s\n", d.ID, d.Name, kind, status)
	}
	return nil
}

// errChecked makes the command exit 1 after printing the problems itself.
var errChecked = errors.New("")

func designCheck(args []string, stdout io.Writer) error {
	if len(args) != 1 {
		return errors.New("usage: skyhud design check <folder>")
	}
	folder := filepath.Clean(args[0])
	d, err := design.Load(os.DirFS(filepath.Dir(folder)), filepath.Base(folder))
	if err != nil {
		return err
	}
	ps := design.Check(d)
	if len(ps) == 0 {
		fmt.Fprintf(stdout, "%s: ok\n", d.Name)
		return nil
	}
	fmt.Fprintf(stdout, "%s: %d problem(s)\n", d.Name, len(ps))
	for _, p := range ps {
		fmt.Fprintf(stdout, "  %s\n", p)
	}
	return errChecked
}

type colorFlags map[string][3]float64

func (c colorFlags) String() string { return "" }

func (c colorFlags) Set(v string) error {
	key, hex, ok := strings.Cut(v, "=")
	rgb, err := parseHex(hex)
	if !ok || err != nil || key == "" {
		return fmt.Errorf("expected key=#rrggbb, got %q", v)
	}
	c[key] = rgb
	return nil
}

// colorList returns a design's colors as name=#rrggbb, main first, then by name.
func colorList(colors map[string][3]float64) []string {
	names := make([]string, 0, len(colors))
	for k := range colors {
		names = append(names, k)
	}
	sort.Slice(names, func(i, j int) bool { return names[i] == "main" || names[j] != "main" && names[i] < names[j] })
	out := []string{}
	for _, k := range names {
		c := colors[k]
		out = append(out, fmt.Sprintf("%s=#%02x%02x%02x", k, hexByte(c[0]), hexByte(c[1]), hexByte(c[2])))
	}
	return out
}

func hexByte(v float64) int { return int(math.Round(math.Max(0, math.Min(1, v)) * 255)) }

func parseHex(s string) ([3]float64, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) != 6 {
		return [3]float64{}, errors.New("bad color")
	}
	var out [3]float64
	for i := range 3 {
		n, err := strconv.ParseUint(s[2*i:2*i+2], 16, 8)
		if err != nil {
			return out, err
		}
		out[i] = math.Round(float64(n)/255*1000) / 1000
	}
	return out, nil
}

var slugBad = regexp.MustCompile(`[^a-z0-9]+`)

func slug(name string) string {
	s := strings.Trim(slugBad.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if s == "" {
		s = "design"
	}
	return s
}

// designSave copies a design under a new name with new colors, font and
// layout: the way to make a design without Fusion.
func designSave(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("design save", flag.ContinueOnError)
	fs.SetOutput(stderr)
	from := fs.String("from", "goggles", "design to start from")
	name := fs.String("name", "", "name of the new design")
	dir := fs.String("dir", "", "the user's designs folder")
	font := fs.String("font", "", "mono, sans or a font name")
	layout := fs.String("layout", "", `piece places as JSON: {"speed": {"x": 0.1, "y": 0.9, "size": 1}}`)
	colors := colorFlags{}
	fs.Var(colors, "color", "key=#rrggbb, e.g. accent=#ff8800 (repeat)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *name == "" || *dir == "" {
		return errors.New("usage: skyhud design save --from ID --name NAME --dir D")
	}
	d, err := design.Find(*from, *dir)
	if err != nil {
		return err
	}
	out := *d
	out.Name, out.Note, out.Builtin = *name, "Based on "+d.Name, false
	out.Colors, out.Pieces = map[string][3]float64{}, map[string]design.PieceDef{}
	for k, v := range d.Colors {
		out.Colors[k] = v
	}
	for k, v := range colors {
		out.Colors[k] = v
	}
	for k, v := range d.Pieces {
		out.Pieces[k] = v
	}
	if *font != "" {
		out.Font = *font
	}
	if *layout != "" {
		var places map[string]design.PieceDef
		if err := json.Unmarshal([]byte(*layout), &places); err != nil {
			return fmt.Errorf("--layout: %w", err)
		}
		for k, v := range places {
			if _, ok := out.Pieces[k]; ok {
				out.Pieces[k] = v
			}
		}
	}
	id := slug(*name)
	builtin := map[string]bool{}
	for _, b := range design.Builtins() {
		builtin[b.ID] = true
	}
	for i := 2; ; i++ { // never overwrite another design, nor hide a built-in one
		if _, err := os.Stat(filepath.Join(*dir, id)); errors.Is(err, os.ErrNotExist) && !builtin[id] {
			break
		}
		id = fmt.Sprintf("%s-%d", slug(*name), i)
	}
	if err := out.Save(filepath.Join(*dir, id)); err != nil {
		return err
	}
	fmt.Fprintln(stdout, id)
	return nil
}

// designPreview writes a comp of the design with a made-up sample flight, to
// load in Fusion while making a design.
func designPreview(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("design preview", flag.ContinueOnError)
	fs.SetOutput(stderr)
	out := fs.String("o", "", "output .comp")
	dir := fs.String("dir", "", "the user's designs folder")
	seconds := fs.Float64("seconds", 30, "length of the sample flight")
	what, err := parse(fs, args)
	if err != nil {
		return err
	}
	if *out == "" {
		return errors.New("missing -o <file.comp>")
	}
	var d *design.Design
	if st, err := os.Stat(filepath.Join(what, "design.json")); err == nil && !st.IsDir() {
		folder := filepath.Clean(what)
		d, err = design.Load(os.DirFS(filepath.Dir(folder)), filepath.Base(folder))
		if err != nil {
			return err
		}
	} else if d, err = design.Find(what, *dir); err != nil {
		return err
	}
	if ps := design.Check(d); len(ps) > 0 {
		return fmt.Errorf("design %s has problems; run skyhud design check", d.Name)
	}
	v, pts := hud.SampleFlight(*seconds, 30)
	f, err := os.Create(*out)
	if err != nil {
		return err
	}
	st := hud.Style{Mono: hud.DefaultFont(runtime.GOOS), Sans: hud.DefaultSans(runtime.GOOS), Aspect: 16.0 / 9, Path: hud.PathPolyline(pts, 16.0/9)}
	if err := hud.WriteComp(f, v, d, hud.DefaultSettings(), st); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
