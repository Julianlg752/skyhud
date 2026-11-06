package djilog

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Julianlg752/skyhud/internal/dji"
	"github.com/Julianlg752/skyhud/internal/secret"
)

// Real logs, compared with dji-log-parser's output:
// SKYHUD_DJI_LOGS=<FlightRecords folder> SKYHUD_DJI_DECODED=<decoded JSON folder>
// SKYHUD_DJI_NETWORK=1 allows keychain requests (cached afterwards).
func TestRealLogs(t *testing.T) {
	logs, decoded := os.Getenv("SKYHUD_DJI_LOGS"), os.Getenv("SKYHUD_DJI_DECODED")
	if logs == "" || decoded == "" {
		t.Skip("SKYHUD_DJI_LOGS / SKYHUD_DJI_DECODED not set")
	}
	cacheDir, _ := os.UserCacheDir()
	cache := KeychainCache{Dir: filepath.Join(cacheDir, "SkyHUD", "keychains")}
	for _, c := range realPairs(t, logs, decoded) {
		t.Run(c.json, func(t *testing.T) {
			b, err := os.ReadFile(filepath.Join(logs, c.log))
			if err != nil {
				t.Skip(err)
			}
			var want struct {
				Details struct {
					AircraftName string `json:"aircraftName"`
				} `json:"details"`
				Frames []struct {
					Custom struct {
						DateTime string `json:"dateTime"`
					} `json:"custom"`
					OSD struct {
						Height, XSpeed, YSpeed float64
						FlyTime                float64 `json:"flyTime"`
					} `json:"osd"`
					RC struct {
						Aileron, Elevator, Throttle, Rudder int
					} `json:"rc"`
					Battery struct {
						ChargeLevel int     `json:"chargeLevel"`
						Voltage     float64 `json:"voltage"`
					} `json:"battery"`
				} `json:"frames"`
			}
			jb, err := os.ReadFile(filepath.Join(decoded, c.json))
			if err != nil {
				t.Skip(err)
			}
			if err := json.Unmarshal(jb, &want); err != nil {
				t.Fatal(err)
			}
			l, err := Open(b)
			if err != nil {
				t.Fatal(err)
			}
			if l.Aircraft != want.Details.AircraftName {
				t.Fatalf("aircraft %q, want %q", l.Aircraft, want.Details.AircraftName)
			}
			var kc [][]KeyPoint
			if l.Version >= 13 {
				id := LogID(b)
				var ok bool
				if kc, ok = cache.Get(id); !ok {
					if os.Getenv("SKYHUD_DJI_NETWORK") != "1" {
						t.Skip("keychains not cached and SKYHUD_DJI_NETWORK not set")
					}
					key, err := secret.System.Get(secret.DJIKey)
					if err != nil {
						t.Skip("no DJI key stored")
					}
					req, err := l.KeychainRequest()
					if err != nil {
						t.Fatal(err)
					}
					if kc, err = FetchKeychains(t.Context(), http.DefaultClient, DefaultEndpoint, key, req); err != nil {
						t.Fatal(err)
					}
					cache.Put(id, kc)
				}
			}
			got, err := l.Frames(kc)
			if err != nil {
				t.Logf("frames: %v", err)
			}
			if len(got) != len(want.Frames) {
				t.Fatalf("%d frames, want %d", len(got), len(want.Frames))
			}
			for i := 0; i < len(got); i += 50 {
				g, w := got[i], want.Frames[i]
				wt, _ := time.Parse(time.RFC3339Nano, w.Custom.DateTime)
				// dji-log-parser shows 1970 before the first time record; SkyHUD leaves it zero.
				bad := (wt.Year() > 1970 && !g.Time.Equal(wt)) ||
					math.Abs(g.Height-w.OSD.Height) > 0.01 || math.Abs(g.XSpeed-w.OSD.XSpeed) > 0.01 || math.Abs(g.YSpeed-w.OSD.YSpeed) > 0.01 ||
					g.Aileron != w.RC.Aileron || g.Elevator != w.RC.Elevator || g.Throttle != w.RC.Throttle || g.Rudder != w.RC.Rudder ||
					g.Battery != w.Battery.ChargeLevel || math.Abs(g.Volts-w.Battery.Voltage) > 0.01 ||
					g.FlyTime >= 0 && math.Abs(g.FlyTime-w.OSD.FlyTime) > 0.01
				if bad {
					t.Fatalf("frame %d:\n got %+v\nwant %+v", i, g, w)
				}
			}
			t.Logf("v%d %s: %d frames match, start %s", l.Version, strings.TrimSpace(l.Aircraft), len(got), l.Start.Format(time.RFC3339))
		})
	}
}

// realPairs pairs each decoded file YYYY-MM-DD_HH-MM.json with the log
// DJIFlightRecord_YYYY-MM-DD_[HH-MM-SS].txt it came from.
func realPairs(t *testing.T, logs, decoded string) []struct{ log, json string } {
	var out []struct{ log, json string }
	files, _ := filepath.Glob(filepath.Join(decoded, "*.json"))
	for _, f := range files {
		name := filepath.Base(f)
		day, hm, ok := strings.Cut(strings.TrimSuffix(name, ".json"), "_")
		if !ok {
			continue
		}
		m, _ := filepath.Glob(filepath.Join(logs, "DJIFlightRecord_"+day+"_\\["+hm+"-*].txt"))
		if len(m) == 1 {
			out = append(out, struct{ log, json string }{filepath.Base(m[0]), name})
		}
	}
	if len(out) == 0 {
		t.Skip("no decoded file matches a log")
	}
	return out
}

// SKYHUD_AVATA=<clip> SKYHUD_DJI_LOGS=<FlightRecords>: the clip's log is
// found and lined up (keychains from the cache filled by TestRealLogs).
// SKYHUD_AVATA_LOG and SKYHUD_AVATA_OFFSET give the expected log name and
// offset in seconds (either sign, within 0.15 s).
func TestRealPair(t *testing.T) {
	clip, logs := os.Getenv("SKYHUD_AVATA"), os.Getenv("SKYHUD_DJI_LOGS")
	if clip == "" || logs == "" {
		t.Skip("SKYHUD_AVATA / SKYHUD_DJI_LOGS not set")
	}
	video, err := dji.Read(clip)
	if err != nil {
		t.Fatal(err)
	}
	cacheDir, _ := os.UserCacheDir()
	cache := KeychainCache{Dir: filepath.Join(cacheDir, "SkyHUD", "keychains")}
	r, err := FindAndAlign(logs, video, Options{Keychains: func(l *Log, raw []byte) ([][]KeyPoint, error) {
		if kc, ok := cache.Get(LogID(raw)); ok {
			return kc, nil
		}
		return nil, errors.New("not cached")
	}})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("log %q offset %.2f s, %d stick points, notes %q", r.Log, r.Offset, len(r.Sticks.T), r.Notes)
	if want := os.Getenv("SKYHUD_AVATA_LOG"); want != "" && r.Log != want {
		t.Fatalf("log %q, want %q", r.Log, want)
	}
	if want, err := strconv.ParseFloat(os.Getenv("SKYHUD_AVATA_OFFSET"), 64); err == nil &&
		math.Abs(r.Offset-want) > 0.15 && math.Abs(r.Offset+want) > 0.15 {
		t.Fatalf("offset %v, want %v", r.Offset, want)
	}
}
