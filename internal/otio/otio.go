// Package otio reads the shots of a timeline from DaVinci Resolve's
// OpenTimelineIO export, including their speed changes and speed ramps.
// Resolve's scripting API reports only one speed per clip; the export holds
// the full time map.
package otio

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"strings"

	"github.com/Julianlg752/skyhud/internal/retime"
)

// Clip is one shot on a video track.
type Clip struct {
	Track       int   // 1-based, counting video tracks only
	RecordStart int64 // timeline frame of the first frame, including the timeline start
	Frames      int64 // length in timeline frames
	Name, Path  string
	Map         retime.Map
	Notes       []string
}

// Timeline is what Read finds in an export.
type Timeline struct {
	Rate  float64 // timeline frames per second
	Start int64   // first timeline frame, e.g. 108000 for 01:00:00:00 at 30 fps
	Clips []Clip
	Notes []string
}

// CurvedNote is added to a clip whose ramp has smoothed points.
const CurvedNote = "smooth speed ramp: the HUD follows it as straight lines"

type rationalTime struct {
	Value float64 `json:"value"`
	Rate  float64 `json:"rate"`
}

func (r rationalTime) seconds() float64 { return r.Value / r.Rate }

type timeRange struct {
	Start    rationalTime `json:"start_time"`
	Duration rationalTime `json:"duration"`
}

type item struct {
	Schema      string                    `json:"OTIO_SCHEMA"`
	Name        string                    `json:"name"`
	Kind        string                    `json:"kind"`
	SourceRange *timeRange                `json:"source_range"`
	Children    []item                    `json:"children"`
	Effects     []effect                  `json:"effects"`
	MediaRefs   map[string]mediaReference `json:"media_references"`
	ActiveRef   string                    `json:"active_media_reference_key"`
	MediaRef    *mediaReference           `json:"media_reference"`
}

type mediaReference struct {
	TargetURL      string     `json:"target_url"`
	AvailableRange *timeRange `json:"available_range"`
}

type effect struct {
	Schema     string   `json:"OTIO_SCHEMA"`
	TimeScalar *float64 `json:"time_scalar"`
	Metadata   struct {
		Resolve struct {
			KeyFrames [][]any `json:"Key Frames"`
		} `json:"Resolve_OTIO"`
	} `json:"metadata"`
}

type document struct {
	Schema      string        `json:"OTIO_SCHEMA"`
	GlobalStart *rationalTime `json:"global_start_time"`
	Tracks      *item         `json:"tracks"`
}

// Read parses an OTIO timeline exported by Resolve.
func Read(r io.Reader) (*Timeline, error) {
	var doc document
	if err := json.NewDecoder(r).Decode(&doc); err != nil {
		return nil, fmt.Errorf("otio: %w", err)
	}
	if doc.Tracks == nil {
		return nil, errors.New("otio: no tracks in the timeline export")
	}
	if doc.GlobalStart == nil || doc.GlobalStart.Rate <= 0 {
		return nil, errors.New("otio: the timeline export has no frame rate")
	}
	tl := &Timeline{Rate: doc.GlobalStart.Rate, Start: int64(math.Round(doc.GlobalStart.Value))}
	video := 0
	for _, tr := range doc.Tracks.Children {
		if tr.Schema != "" && !strings.HasPrefix(tr.Schema, "Track.") || tr.Kind != "Video" {
			continue
		}
		video++
		tl.readTrack(video, tr)
	}
	return tl, nil
}

func (tl *Timeline) frames(seconds float64) int64 { return int64(math.Round(seconds * tl.Rate)) }

func (tl *Timeline) readTrack(n int, tr item) {
	pos := tl.Start
	for _, it := range tr.Children {
		if strings.HasPrefix(it.Schema, "Transition.") || it.SourceRange == nil || it.SourceRange.Duration.Rate <= 0 {
			continue
		}
		length := tl.frames(it.SourceRange.Duration.seconds())
		switch {
		case strings.HasPrefix(it.Schema, "Clip."):
			c := Clip{Track: n, RecordStart: pos, Frames: length, Name: it.Name, Map: retime.Map{Speed: 1}}
			ref := it.reference()
			if ref != nil {
				c.Path = path(ref.TargetURL)
			}
			start := it.SourceRange.Start
			if start.Rate > 0 {
				c.Map.Start = start.seconds()
				if ref != nil && ref.AvailableRange != nil && ref.AvailableRange.Start.Rate > 0 {
					c.Map.Start -= ref.AvailableRange.Start.seconds()
				}
			}
			c.readEffects(it.Effects)
			tl.Clips = append(tl.Clips, c)
		case strings.HasPrefix(it.Schema, "Stack."):
			tl.Notes = append(tl.Notes, fmt.Sprintf("compound clip %q on video track %d is skipped", it.Name, n))
		}
		pos += length
	}
}

func (it item) reference() *mediaReference {
	if r, ok := it.MediaRefs[it.ActiveRef]; ok {
		return &r
	}
	for _, r := range it.MediaRefs {
		return &r
	}
	return it.MediaRef
}

func (c *Clip) readEffects(effects []effect) {
	for _, e := range effects {
		switch {
		case strings.HasPrefix(e.Schema, "LinearTimeWarp.") && e.TimeScalar != nil:
			c.Map.Speed = *e.TimeScalar
		case strings.HasPrefix(e.Schema, "FreezeFrame."):
			c.Map.Speed = 0
		case strings.HasPrefix(e.Schema, "TimeEffect."):
			curved := false
			for _, k := range e.Metadata.Resolve.KeyFrames {
				if len(k) < 2 {
					continue
				}
				clip, ok1 := k[0].(float64)
				src, ok2 := k[1].(float64)
				if !ok1 || !ok2 {
					continue
				}
				c.Map.Keys = append(c.Map.Keys, retime.Key{Clip: clip, Source: src})
				if len(k) > 2 && k[2] == true {
					curved = true
				}
			}
			if curved {
				c.Notes = append(c.Notes, CurvedNote)
			}
		}
	}
}

// path turns a target_url into a file path.
func path(target string) string {
	if !strings.HasPrefix(target, "file://") {
		return target
	}
	p := strings.TrimPrefix(target, "file://")
	if u, err := url.PathUnescape(p); err == nil {
		p = u
	}
	// file:///C:/clips/a.mp4 -> C:/clips/a.mp4
	if len(p) > 2 && p[0] == '/' && p[2] == ':' {
		p = p[1:]
	}
	return p
}

// Find returns the clip on a video track that starts at a timeline frame.
func (tl *Timeline) Find(track int, recordStart int64) (Clip, bool) {
	for _, c := range tl.Clips {
		if c.Track == track && c.RecordStart == recordStart {
			return c, true
		}
	}
	return Clip{}, false
}
