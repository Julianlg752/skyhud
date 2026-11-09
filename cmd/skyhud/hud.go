package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/Julianlg752/skyhud/internal/design"
	"github.com/Julianlg752/skyhud/internal/djilog"
	"github.com/Julianlg752/skyhud/internal/hud"
	"github.com/Julianlg752/skyhud/internal/otio"
	"github.com/Julianlg752/skyhud/internal/plugin"
	"github.com/Julianlg752/skyhud/internal/retime"
	"github.com/Julianlg752/skyhud/internal/secret"
	"github.com/Julianlg752/skyhud/internal/telemetry"
)

// jobsFile is what the Resolve panel asks for: one HUD comp per shot.
type jobsFile struct {
	Version  int       `json:"version"`
	FPS      float64   `json:"fps"`    // timeline frames per second
	Aspect   float64   `json:"aspect"` // timeline width / height
	OTIO     string    `json:"otio"`   // the timeline's OTIO export, optional
	Settings *settings `json:"settings"`
	Shots    []shot    `json:"shots"`
}

// settings may name only some pieces; the rest keep their defaults.
type settings struct {
	Logs    string                `json:"logs"`    // DJI flight logs folder
	Design  string                `json:"design"`  // design id, default goggles
	Designs string                `json:"designs"` // the user's designs folder
	Show    map[hud.Piece]bool    `json:"show"`
	Height  hud.HeightMode        `json:"height"`
	Units   hud.Units             `json:"units"` // metric (default) or imperial
	Scale   float64               `json:"scale"`
	Colors  map[string][3]float64 `json:"colors"`
	Font    string                `json:"font"`
	// Colors and font picked in the panel, per design id.
	DesignColors map[string]map[string][3]float64 `json:"designColors"`
	DesignFonts  map[string]string                `json:"designFonts"`
	Layout       map[hud.Piece]hud.Place          `json:"layout"` // Plans 2-3 (Text+ sizes): ignored
	// Layouts per design, from the panel's "Use this shot's layout for all".
	Layouts       map[string]map[hud.Piece]hud.Place `json:"layouts"`
	LayoutVersion int                                `json:"layoutVersion"`
}

type shot struct {
	ID          string  `json:"id"`
	File        string  `json:"file"`
	Track       int     `json:"track"`
	RecordStart int64   `json:"recordStart"`
	Frames      int     `json:"frames"`
	Speed       float64 `json:"speed"`       // percent, from the scripting API
	SourceStart float64 `json:"sourceStart"` // seconds into the file, from the scripting API
	Nudge       float64 `json:"nudge"`       // seconds added to the data time
	Out         string  `json:"out"`
}

// merge returns the HUD settings for design id. Layouts saved before
// layoutVersion 2 measured Text+ sizes, so they are ignored.
func (s *settings) merge(id string) hud.Settings {
	out := hud.DefaultSettings()
	if s == nil {
		return out
	}
	for p, v := range s.Show {
		out.Show[p] = v
	}
	if s.Height == hud.SeaLevel || s.Height == hud.FromStart {
		out.Height = s.Height
	}
	if s.Scale > 0 {
		out.Scale = s.Scale
	}
	if s.Units == hud.Imperial {
		out.Units = hud.Imperial
	}
	if s.LayoutVersion >= 2 {
		for p, v := range s.Layouts[id] {
			out.Layout[p] = v
		}
	}
	return out
}

// chooseDesign returns the design the settings ask for, or Goggles with a
// note when it is missing or fails the safety check.
func chooseDesign(s *settings) (*design.Design, string, error) {
	id, dir := "goggles", ""
	if s != nil {
		dir = s.Designs
		if s.Design != "" {
			id = s.Design
		}
	}
	d, err := design.Find(id, dir)
	note := ""
	if err == nil {
		if ps := design.Check(d); len(ps) > 0 {
			err = fmt.Errorf("design %s was refused: %s", d.Name, ps[0])
		}
	}
	if err != nil {
		note = err.Error() + "; using Goggles"
		d, err = design.Find("goggles", "")
	}
	return d, note, err
}

func hudCommand(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("hud", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jobsPath := fs.String("jobs", "", "jobs file written by the Resolve panel")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *jobsPath == "" {
		return errors.New("missing --jobs <file.json>")
	}
	b, err := os.ReadFile(*jobsPath)
	if err != nil {
		return err
	}
	var jobs jobsFile
	if err := json.Unmarshal(b, &jobs); err != nil {
		return fmt.Errorf("jobs file %s: %w", *jobsPath, err)
	}
	if jobs.FPS <= 0 {
		return fmt.Errorf("jobs file %s: no frame rate", *jobsPath)
	}
	var tl *otio.Timeline
	if jobs.OTIO != "" {
		if f, err := os.Open(jobs.OTIO); err == nil {
			tl, _ = otio.Read(f) // without it, shots fall back to the API's numbers
			f.Close()
		}
	}
	logs := ""
	d, designNote, err := chooseDesign(jobs.Settings)
	if err != nil {
		return err
	}
	s := jobs.Settings.merge(d.ID)
	if s.Units == hud.Imperial && !d.HasUnits() {
		designNote = strings.TrimPrefix(designNote+"; design "+d.Name+" shows fixed km/h and m labels, so it stays metric (the design guide shows how to add mph and ft)", "; ")
	}
	style := hud.Style{Mono: hud.DefaultFont(runtime.GOOS), Sans: hud.DefaultSans(runtime.GOOS), Aspect: jobs.Aspect}
	if jobs.Settings != nil {
		logs = jobs.Settings.Logs
		style.Colors, style.Font = jobs.Settings.Colors, jobs.Settings.Font
		if c, ok := jobs.Settings.DesignColors[d.ID]; ok {
			style.Colors = c
		}
		if f := jobs.Settings.DesignFonts[d.ID]; f != "" {
			style.Font = f
		}
	}
	src := &sources{tracks: map[string]*telemetry.Track{}, failed: map[string]error{}, logNotes: map[string]string{}, logs: logs}
	for _, sh := range jobs.Shots {
		note, flips, err := buildShot(sh, jobs.FPS, tl, d, s, style, src)
		if err != nil {
			fmt.Fprintf(stdout, "skip\t%s\t%s\t\n", field(sh.ID), field(err.Error()))
			continue
		}
		if designNote != "" {
			note = strings.TrimSuffix(designNote+"; "+note, "; ")
		}
		var at []string
		for _, f := range flips {
			at = append(at, strconv.FormatFloat(f, 'f', -1, 64))
		}
		fmt.Fprintf(stdout, "ok\t%s\t%s\tflips=%s\n", field(sh.ID), field(note), strings.Join(at, ","))
	}
	return nil
}

// buildShot writes one shot's comp; it returns a note for the panel and
// the flips in the shot (seconds from its start).
func buildShot(sh shot, fps float64, tl *otio.Timeline, d *design.Design, s hud.Settings, st hud.Style, src *sources) (string, []float64, error) {
	var notes []string
	m := retime.Map{Start: sh.SourceStart, Speed: sh.Speed / 100}
	c, found := otio.Clip{}, false
	if tl != nil {
		c, found = tl.Find(sh.Track, sh.RecordStart)
	}
	if found {
		if c.Path != "" && !strings.EqualFold(filepath.Base(c.Path), filepath.Base(sh.File)) {
			return "", nil, fmt.Errorf("the timeline export shows %s at this shot, not %s; run it again", filepath.Base(c.Path), filepath.Base(sh.File))
		}
		m = c.Map
		if m.Ramped() {
			notes = append(notes, "follows a speed ramp")
		}
		notes = append(notes, c.Notes...)
	} else {
		notes = append(notes, "timeline export not found for this shot; using its in point and one constant speed")
	}

	tr, err := src.track(sh.File)
	if err != nil {
		return "", nil, err
	}
	if n := src.logNotes[sh.File]; n != "" {
		notes = append(notes, n)
	}

	v := hud.Sample(tr, m, sh.Frames, fps, sh.Nudge, s.Height)
	if err := os.MkdirAll(filepath.Dir(sh.Out), 0o755); err != nil {
		return "", nil, err
	}
	f, err := os.Create(sh.Out)
	if err != nil {
		return "", nil, err
	}
	st.GForceEstimated = tr.Channels[telemetry.GForce].Estimated
	st.Path = hud.PathPolyline(hud.PathPoints(tr, pathPoints), st.Aspect)
	if err := hud.WriteComp(f, v, d, s, st); err != nil {
		f.Close()
		return "", nil, err
	}
	if err := f.Close(); err != nil {
		return "", nil, err
	}
	return strings.Join(notes, "; "), shotFlips(tr, m, sh.Frames, fps, sh.Nudge), nil
}

// pathPoints is how many points draw the flight path.
const pathPoints = 150

// field keeps a value on one line of the tab-separated output.
func field(s string) string {
	return strings.NewReplacer("\t", " ", "\r", " ", "\n", " ").Replace(s)
}

// installCommand copies this binary to SkyHUD's own folder and puts the
// SkyHUD panel into Resolve's Scripts menu, pointing it at the copy (or at
// this binary with --here, for development).
func installCommand(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("dir", "", "Resolve scripts folder (default: Resolve's Utility scripts folder)")
	here := fs.Bool("here", false, "point the panel at this binary instead of installing a copy")
	if err := fs.Parse(args); err != nil {
		return err
	}
	bin, err := os.Executable()
	if err != nil {
		return err
	}
	if real, err := filepath.EvalSymlinks(bin); err == nil {
		bin = real
	}
	home, _ := os.UserHomeDir()
	if !*here {
		programData := os.Getenv("PROGRAMDATA")
		if programData == "" {
			programData = `C:\ProgramData`
		}
		dest := plugin.BinPath(runtime.GOOS, home, programData)
		if !samePath(bin, dest) {
			if err := plugin.CopyBinary(bin, dest); err != nil {
				return fmt.Errorf("copy skyhud to %s: %w", dest, err)
			}
		}
		bin = dest
	}
	if *dir == "" {
		*dir = plugin.ScriptsDir(runtime.GOOS, home, os.Getenv("APPDATA"))
	}
	path, err := plugin.Install(*dir, bin)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "installed %s\nrestart Resolve, then use Workspace > Scripts > SkyHUD\n", path)
	if w := plugin.PathWarning(runtime.GOOS, bin); w != "" {
		fmt.Fprintln(stdout, w)
	}
	return nil
}

// sources reads each file once and, for DJI clips, adds sticks and battery
// from the matching flight log.
type sources struct {
	tracks   map[string]*telemetry.Track
	failed   map[string]error
	logNotes map[string]string
	logs     string
}

func (src *sources) track(file string) (*telemetry.Track, error) {
	if err, ok := src.failed[file]; ok {
		return nil, err
	}
	if tr, ok := src.tracks[file]; ok {
		return tr, nil
	}
	tr, _, err := load(file, "", -1)
	if err != nil {
		err = fmt.Errorf("%s: %w", filepath.Base(file), err)
		src.failed[file] = err
		return nil, err
	}
	if tr.Source == "dji" && src.logs != "" {
		src.logNotes[file] = addFlightLog(tr, src.logs)
	}
	src.tracks[file] = tr
	return tr, nil
}

// Where fetched keychains are cached, and DJI's endpoint; tests swap them.
var (
	keychainCacheDir string
	keychainEndpoint = djilog.DefaultEndpoint
)

// addFlightLog merges sticks and battery from the matching log into tr and
// returns a note for the panel.
func addFlightLog(tr *telemetry.Track, folder string) string {
	r, err := matchLog(tr, folder)
	if err != nil {
		return err.Error()
	}
	if r.Log == "" {
		return strings.Join(r.Notes, "; ")
	}
	tr.Channels[telemetry.Sticks] = r.Sticks
	tr.Channels[telemetry.Battery] = r.Battery
	return fmt.Sprintf("flight log %s (offset %+.2f s)", r.Log, r.Offset)
}

// matchLog finds the flight log in folder for the DJI clip tr, fetching (and
// caching) keychains for encrypted logs with the stored DJI key.
func matchLog(tr *telemetry.Track, folder string) (djilog.Result, error) {
	dir := keychainCacheDir
	if dir == "" {
		base, err := os.UserCacheDir()
		if err != nil {
			base = os.TempDir()
		}
		dir = filepath.Join(base, "SkyHUD", "keychains")
	}
	cache := djilog.KeychainCache{Dir: dir}
	keys := func(l *djilog.Log, raw []byte) ([][]djilog.KeyPoint, error) {
		id := djilog.LogID(raw)
		if kc, ok := cache.Get(id); ok {
			return kc, nil
		}
		key, err := keyStore.Get(secret.DJIKey)
		if errors.Is(err, secret.ErrNotFound) {
			return nil, errors.New("no DJI key stored")
		}
		if err != nil {
			return nil, err
		}
		req, err := l.KeychainRequest()
		if err != nil {
			return nil, err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		kc, err := djilog.FetchKeychains(ctx, http.DefaultClient, keychainEndpoint, key, req)
		if err != nil {
			return nil, err
		}
		cache.Put(id, kc)
		return kc, nil
	}
	return djilog.FindAndAlign(folder, tr, djilog.Options{Keychains: keys})
}

// shotFlips returns when each flip starts, in seconds from the shot's start.
func shotFlips(tr *telemetry.Track, m retime.Map, frames int, fps, nudge float64) []float64 {
	var out []float64
	flips := hud.Flips(tr)
	for i := 1; i < frames && len(flips) > 0; i++ {
		a := m.SourceAt(float64(i-1)/fps) + nudge
		b := m.SourceAt(float64(i)/fps) + nudge
		for _, f := range flips {
			if f > a && f <= b || i == 1 && f == a {
				out = append(out, math.Round(float64(i)/fps*1000)/1000)
			}
		}
	}
	return out
}
