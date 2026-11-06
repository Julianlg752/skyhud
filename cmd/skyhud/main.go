// Command skyhud reads flight telemetry from video files and writes HUD data
// files for the Resolve plugin.
//
//	skyhud inspect [--json] <clip>
//	skyhud export <clip> -o <file.hud.json> [--keep-location] [--trim-start S]
//	skyhud hud --jobs <jobs.json>
//	skyhud install [--dir D] [--here]
//	skyhud key set|status
//	skyhud design list|check|save|preview
//	skyhud scan <folder> [--logs D] [--json]
//	skyhud version
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/Julianlg752/skyhud/internal/dji"
	"github.com/Julianlg752/skyhud/internal/gopro"
	"github.com/Julianlg752/skyhud/internal/gyroflow"
	"github.com/Julianlg752/skyhud/internal/mp4"
	"github.com/Julianlg752/skyhud/internal/telemetry"
)

const usage = `usage:
  skyhud inspect [--json] [--original GX010123.MP4] <clip>
  skyhud export <clip> -o <file.hud.json> [--keep-location] [--trim-start S] [--original GX010123.MP4]
  skyhud hud --jobs <jobs.json>
  skyhud install [--dir D] [--here]
  skyhud key set|status   (set reads the DJI API key from standard input)
  skyhud design list|check|save|preview
  skyhud scan <folder> [--logs D] [--json]
  skyhud version`

// version is set by scripts/release.sh.
var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, usage)
		return 1
	}
	var err error
	switch args[0] {
	case "inspect":
		err = inspect(args[1:], stdout, stderr)
	case "export":
		err = export(args[1:], stderr)
	case "hud":
		err = hudCommand(args[1:], stdout, stderr)
	case "install":
		err = installCommand(args[1:], stdout, stderr)
	case "key":
		err = keyCommand(args[1:], stdout)
	case "design":
		err = designCommand(args[1:], stdout, stderr)
	case "scan":
		err = scanCommand(args[1:], stdout)
	case "version":
		fmt.Fprintln(stdout, "skyhud", version)
	default:
		err = fmt.Errorf("unknown command %q\n%s", args[0], usage)
	}
	if err != nil {
		if err.Error() != "" {
			fmt.Fprintln(stderr, err)
		}
		return 1
	}
	return 0
}

// parse lets flags come before or after the clip path.
func parse(fs *flag.FlagSet, args []string) (string, error) {
	var clip string
	for {
		if err := fs.Parse(args); err != nil {
			return "", err
		}
		if fs.NArg() == 0 {
			break
		}
		if clip != "" {
			return "", fmt.Errorf("expected one clip, got %q and %q", clip, fs.Arg(0))
		}
		clip = fs.Arg(0)
		args = fs.Args()[1:]
	}
	if clip == "" {
		return "", errors.New("missing clip path\n" + usage)
	}
	return clip, nil
}

// stabilizedInfo describes the link from a Gyroflow export to its original.
type stabilizedInfo struct {
	Original    string  `json:"original"`
	TrimStart   float64 `json:"trimStart"`
	Uncertainty float64 `json:"uncertainty"`
}

var errNoData = errors.New("no GoPro or DJI flight data in this file")

const noTelemetryHint = "; if this is a Gyroflow export, keep the original camera file (GXnnnnnn.MP4 or DJI_..._D.MP4) next to it or pass --original"

// load reads a clip's telemetry. For a Gyroflow export it reads the original
// (found next to it, or given as origPath) and shifts times so 0 is the
// export's first frame. trimStart < 0 means estimate it.
func load(clip, origPath string, trimStart float64) (*telemetry.Track, *stabilizedInfo, error) {
	if origPath == "" {
		if !gyroflow.IsStabilized(clip) {
			return readCamera(clip)
		}
		var err error
		if origPath, err = gyroflow.FindOriginal(clip); err != nil {
			return nil, nil, err
		}
	}
	stab, err := mp4.Open(clip)
	if err != nil {
		return nil, nil, err
	}
	defer stab.Close()
	orig, err := mp4.Open(origPath)
	if err != nil {
		return nil, nil, err
	}
	trim := gyroflow.EstimateTrim(stab, orig)
	orig.Close()
	if trimStart >= 0 {
		trim = gyroflow.Trim{Start: trimStart}
	}
	tr, _, err := readCamera(origPath)
	if err != nil {
		return nil, nil, err
	}
	shift(tr, trim.Start, stab.Duration)
	tr.Start = tr.Start.Add(time.Duration(trim.Start * float64(time.Second)))
	tr.Duration = stab.Duration
	return tr, &stabilizedInfo{Original: filepath.Base(origPath), TrimStart: trim.Start, Uncertainty: trim.Uncertainty}, nil
}

// readCamera reads a camera file with the reader for the data track it holds.
func readCamera(path string) (*telemetry.Track, *stabilizedInfo, error) {
	f, err := mp4.Open(path)
	if err != nil {
		return nil, nil, err
	}
	_, isDJI := f.TrackByFormat("djmd")
	f.Close()
	if isDJI {
		tr, err := dji.Read(path)
		return tr, nil, err
	}
	tr, err := gopro.Read(path)
	if errors.Is(err, gopro.ErrNoTelemetry) {
		err = fmt.Errorf("%w%s", errNoData, noTelemetryHint)
	}
	return tr, nil, err
}

// shift keeps points inside [from, from+length] and moves them to start at 0.
func shift(tr *telemetry.Track, from, length float64) {
	var gaps [][2]float64
	for _, g := range tr.NoGPS {
		a, b := max(g[0], from)-from, min(g[1], from+length)-from
		if b > a {
			gaps = append(gaps, [2]float64{a, b})
		}
	}
	tr.NoGPS = gaps
	for name, s := range tr.Channels {
		var out telemetry.Series
		out.Estimated = s.Estimated
		for i, t := range s.T {
			if t >= from && t <= from+length {
				out.T = append(out.T, t-from)
				out.V = append(out.V, s.V[i])
			}
		}
		tr.Channels[name] = out
	}
}

type channelInfo struct {
	Points    int     `json:"points"`
	Rate      float64 `json:"rate"`
	Estimated bool    `json:"estimated"`
}

type inspectReport struct {
	Source     string                 `json:"source"`
	Device     string                 `json:"device"`
	Start      string                 `json:"start"`
	Duration   float64                `json:"duration"`
	Channels   map[string]channelInfo `json:"channels"`
	Notes      []string               `json:"notes"`
	Stabilized *stabilizedInfo        `json:"stabilized,omitempty"`
}

func inspect(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("inspect", flag.ContinueOnError)
	fs.SetOutput(stderr)
	asJSON := fs.Bool("json", false, "print JSON")
	orig := fs.String("original", "", "original GoPro file for a stabilized or renamed clip")
	clip, err := parse(fs, args)
	if err != nil {
		return err
	}
	tr, stab, err := load(clip, *orig, -1)
	if err != nil {
		return err
	}
	rep := inspectReport{
		Source: tr.Source, Device: tr.Device, Start: tr.Start.UTC().Format(time.RFC3339),
		Duration: tr.Duration, Channels: map[string]channelInfo{}, Notes: tr.AllNotes(), Stabilized: stab,
	}
	for name, s := range tr.Channels {
		if name == telemetry.GPS {
			continue // positions are never printed
		}
		ci := channelInfo{Points: len(s.T), Estimated: s.Estimated}
		if tr.Duration > 0 {
			ci.Rate = float64(len(s.T)) / tr.Duration
		}
		rep.Channels[name] = ci
	}
	if *asJSON {
		return json.NewEncoder(stdout).Encode(rep)
	}
	fmt.Fprintf(stdout, "%s %s, %.1f s, recorded %s\n", rep.Source, rep.Device, rep.Duration, rep.Start)
	if stab != nil {
		fmt.Fprintf(stdout, "stabilized clip: data from %s, starting %.0f s in (±%.0f s)\n", stab.Original, stab.TrimStart, stab.Uncertainty)
	}
	names := make([]string, 0, len(rep.Channels))
	for n := range rep.Channels {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		c := rep.Channels[n]
		est := ""
		if c.Estimated {
			est = " (estimated)"
		}
		fmt.Fprintf(stdout, "  %-12s %6d points, %6.1f per second%s\n", n, c.Points, c.Rate, est)
	}
	for _, n := range rep.Notes {
		fmt.Fprintf(stdout, "note: %s\n", n)
	}
	return nil
}

func export(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	fs.SetOutput(stderr)
	out := fs.String("o", "", "output HUD data file")
	keep := fs.Bool("keep-location", false, "include absolute GPS positions")
	trimStart := fs.Float64("trim-start", -1, "seconds into the original where a stabilized clip starts (default: estimate)")
	orig := fs.String("original", "", "original GoPro file for a stabilized or renamed clip")
	clip, err := parse(fs, args)
	if err != nil {
		return err
	}
	if *out == "" {
		return errors.New("missing -o <file.hud.json>")
	}
	for _, src := range []string{clip, *orig} {
		if src != "" && samePath(*out, src) {
			return fmt.Errorf("-o %s would overwrite the video; choose a .hud.json file", *out)
		}
	}
	tr, _, err := load(clip, *orig, *trimStart)
	if err != nil {
		return err
	}
	f, err := os.Create(*out)
	if err != nil {
		return err
	}
	if err := tr.WriteHUDFile(f, *keep); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// samePath reports whether two paths name the same file.
func samePath(a, b string) bool {
	if sa, err := os.Stat(a); err == nil {
		if sb, err := os.Stat(b); err == nil {
			return os.SameFile(sa, sb)
		}
	}
	aa, _ := filepath.Abs(a)
	bb, _ := filepath.Abs(b)
	return aa == bb
}
