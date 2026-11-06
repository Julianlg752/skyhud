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
	"strings"
	"time"

	"github.com/Julianlg752/skyhud/internal/gyroflow"
	"github.com/Julianlg752/skyhud/internal/hud"
	"github.com/Julianlg752/skyhud/internal/retime"
	"github.com/Julianlg752/skyhud/internal/telemetry"
)

// scanVideo is what skyhud knows about one video. It never holds positions
// or serial numbers.
type scanVideo struct {
	File     string   `json:"file"`
	Source   string   `json:"source,omitempty"` // gopro or dji
	Device   string   `json:"device,omitempty"`
	Duration float64  `json:"duration,omitempty"`
	Recorded string   `json:"recorded,omitempty"` // camera clock
	Pieces   []string `json:"pieces,omitempty"`   // HUD pieces with data
	Original string   `json:"original,omitempty"` // for a Gyroflow export
	Log      string   `json:"log,omitempty"`      // matched DJI flight log
	Offset   *float64 `json:"offset,omitempty"`   // s, video time = log time + offset
	Problem  string   `json:"problem,omitempty"`
	Notes    []string `json:"notes,omitempty"`

	start time.Time // DJI clips only
}

type scanReport struct {
	Folder string      `json:"folder"`
	Videos []scanVideo `json:"videos"`
	Gaps   []string    `json:"gaps"` // videos without data, logs without a video
}

// Pieces the scan reports; the flip comes with the horizon.
var scanPieces = []hud.Piece{hud.Speed, hud.Height, hud.GForce, hud.Horizon, hud.Path, hud.Sticks, hud.Battery}

func scanCommand(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("scan", flag.ContinueOnError)
	logs := fs.String("logs", "", "folder with DJI flight logs (DJIFlightRecord_*.txt)")
	asJSON := fs.Bool("json", false, "print JSON")
	folder, err := parse(fs, args)
	if err != nil {
		return err
	}
	r, err := scan(folder, *logs)
	if err != nil {
		return err
	}
	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(r)
	}
	writeScan(stdout, r)
	return nil
}

func scan(folder, logs string) (scanReport, error) {
	entries, err := os.ReadDir(folder)
	if err != nil {
		return scanReport{}, err
	}
	r := scanReport{Folder: folder, Videos: []scanVideo{}, Gaps: []string{}}
	used := map[string]bool{}
	var djiStarts []time.Time
	for _, e := range entries {
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if e.IsDir() || (ext != ".mp4" && ext != ".mov") || strings.HasPrefix(e.Name(), "._") {
			continue
		}
		v := scanOne(filepath.Join(folder, e.Name()), logs)
		if v.Log != "" {
			used[v.Log] = true
		}
		if !v.start.IsZero() {
			djiStarts = append(djiStarts, v.start)
		}
		if v.Problem != "" {
			r.Gaps = append(r.Gaps, v.File+": "+v.Problem)
		}
		r.Videos = append(r.Videos, v)
	}
	sort.Slice(r.Videos, func(i, j int) bool { return r.Videos[i].File < r.Videos[j].File })
	if logs != "" {
		entries, err := os.ReadDir(logs)
		if err != nil {
			return r, fmt.Errorf("flight logs folder: %w", err)
		}
		for _, e := range entries {
			n := e.Name()
			if strings.HasPrefix(n, "DJIFlightRecord_") && strings.HasSuffix(n, ".txt") && !used[n] && nearAny(n, djiStarts) {
				r.Gaps = append(r.Gaps, n+": no video in this folder matches this flight log")
			}
		}
	}
	return r, nil
}

func scanOne(path, logs string) scanVideo {
	v := scanVideo{File: filepath.Base(path)}
	tr, stab, err := load(path, "", -1)
	switch {
	case errors.Is(err, gyroflow.ErrOriginalMissing):
		v.Problem = "Gyroflow export, but the original camera file is not next to it"
		return v
	case errors.Is(err, errNoData):
		v.Problem = "no flight data (not a GoPro or DJI camera file)"
		return v
	case err != nil:
		v.Problem = err.Error()
		return v
	}
	v.Source, v.Device, v.Duration = tr.Source, tr.Device, tr.Duration
	if !tr.Start.IsZero() {
		v.Recorded = tr.Start.Local().Format("2006-01-02 15:04:05")
	}
	if tr.Source == "dji" {
		v.start = tr.Start
	}
	if stab != nil {
		v.Original = stab.Original
	}
	if tr.Source == "dji" && logs != "" {
		m, err := matchLog(tr, logs)
		switch {
		case err != nil:
			v.Notes = append(v.Notes, err.Error())
		case m.Log == "":
			v.Notes = append(v.Notes, m.Notes...)
		default:
			tr.Channels[telemetry.Sticks] = m.Sticks
			tr.Channels[telemetry.Battery] = m.Battery
			off := m.Offset
			v.Log, v.Offset = m.Log, &off
		}
	}
	const fps = 2
	frames := max(1, int(tr.Duration*fps))
	vals := hud.Sample(tr, retime.Map{Speed: 1}, frames, fps, 0, hud.FromStart)
	for _, p := range scanPieces {
		for _, ok := range vals[p].OK {
			if ok {
				v.Pieces = append(v.Pieces, string(p))
				break
			}
		}
	}
	if len(v.Pieces) == 0 {
		v.Problem = "no usable flight data in this clip"
	}
	v.Notes = append(v.Notes, tr.AllNotes()...)
	return v
}

// nearAny reports whether the flight log named name (its date is the
// phone's local date) is within a day of one of the clips. Logs from other
// days are other flights, not gaps.
func nearAny(name string, starts []time.Time) bool {
	day, err := time.Parse("2006-01-02", strings.TrimPrefix(name, "DJIFlightRecord_")[:min(10, len(name)-16)])
	if err != nil {
		return false
	}
	for _, s := range starts {
		if d := s.Sub(day); d > -36*time.Hour && d < 60*time.Hour {
			return true
		}
	}
	return false
}

func writeScan(w io.Writer, r scanReport) {
	if len(r.Videos) == 0 {
		fmt.Fprintf(w, "No videos (.mp4 or .mov) in %s\n", r.Folder)
		return
	}
	fmt.Fprintf(w, "%d video(s) in %s\n", len(r.Videos), r.Folder)
	for _, v := range r.Videos {
		head := []string{v.File}
		if v.Device != "" {
			head = append(head, v.Device)
		}
		if v.Duration > 0 {
			head = append(head, fmt.Sprintf("%.1f s", v.Duration))
		}
		if v.Recorded != "" {
			head = append(head, v.Recorded)
		}
		fmt.Fprintf(w, "\n%s\n", strings.Join(head, " · "))
		if v.Problem != "" {
			fmt.Fprintf(w, "  problem: %s\n", v.Problem)
		}
		if len(v.Pieces) > 0 {
			fmt.Fprintf(w, "  data: %s\n", strings.Join(v.Pieces, ", "))
		}
		if v.Original != "" {
			fmt.Fprintf(w, "  Gyroflow export of %s\n", v.Original)
		}
		if v.Log != "" {
			fmt.Fprintf(w, "  flight log: %s (offset %+.2f s)\n", v.Log, *v.Offset)
		}
		for _, n := range v.Notes {
			fmt.Fprintf(w, "  note: %s\n", n)
		}
	}
	if len(r.Gaps) > 0 {
		fmt.Fprintf(w, "\nGaps:\n")
		for _, g := range r.Gaps {
			fmt.Fprintf(w, "  %s\n", g)
		}
	}
}
