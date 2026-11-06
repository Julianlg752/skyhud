package telemetry

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func sampleTrack() *Track {
	return &Track{
		Source:   "gopro",
		Device:   "HERO10 Black",
		Start:    time.Date(2024, 5, 4, 14, 0, 0, 0, time.UTC),
		Duration: 2,
		Channels: map[string]Series{
			"gps":  {T: []float64{0, 1}, V: [][]float64{{40.1234567, -76.0123456}, {40.1235567, -76.0123456}}},
			"path": {T: []float64{0, 1}, V: [][]float64{{0, 0}, {0, 11.1}}},
		},
	}
}

func TestWriteHUDFileDropsLocation(t *testing.T) {
	var buf bytes.Buffer
	if err := sampleTrack().WriteHUDFile(&buf, false); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Contains(out, "40.123") || strings.Contains(out, `"gps"`) {
		t.Fatalf("location leaked: %s", out)
	}
	var f struct {
		Format   string                     `json:"format"`
		Version  int                        `json:"version"`
		Device   string                     `json:"device"`
		Start    string                     `json:"start"`
		Channels map[string]json.RawMessage `json:"channels"`
	}
	if err := json.Unmarshal(buf.Bytes(), &f); err != nil {
		t.Fatal(err)
	}
	if f.Format != "skyhud-hud" || f.Version != 1 || f.Device != "HERO10 Black" || f.Start != "2024-05-04T14:00:00Z" {
		t.Fatalf("header = %+v", f)
	}
	if _, ok := f.Channels["path"]; !ok {
		t.Fatal("path missing")
	}
}

func TestWriteHUDFileKeepsLocationWhenAsked(t *testing.T) {
	var buf bytes.Buffer
	if err := sampleTrack().WriteHUDFile(&buf, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"gps"`) {
		t.Fatal("gps missing with keepLocation")
	}
}
