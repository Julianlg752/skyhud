// Package gyroflow links a Gyroflow-stabilized clip, which has no telemetry,
// back to the original camera file (GoPro or DJI) and estimates where the
// export starts.
package gyroflow

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/Julianlg752/skyhud/internal/mp4"
)

// ErrOriginalMissing means the original file isn't next to the stabilized clip.
var ErrOriginalMissing = errors.New("original camera file not found")

type originalMissing struct{ name string }

func (e originalMissing) Error() string {
	return "the original camera file " + e.name + " was not found next to the stabilized clip; it holds the flight data"
}

func (e originalMissing) Is(target error) bool { return target == ErrOriginalMissing }

// IsStabilized reports whether path looks like a Gyroflow export: its name
// says so, or it has no telemetry while an original sits next to it.
func IsStabilized(path string) bool {
	if strings.Contains(strings.ToLower(filepath.Base(path)), "_stabilized") {
		return true
	}
	f, err := mp4.Open(path)
	if err != nil {
		return false
	}
	_, gopro := f.TrackByFormat("gpmd")
	_, dji := f.TrackByFormat("djmd")
	hasTelemetry := gopro || dji
	f.Close()
	if hasTelemetry {
		return false
	}
	_, err = FindOriginal(path)
	return err == nil
}

// FindOriginal returns the original clip for a stabilized one. It tries the
// name before "_stabilized" (DJI_20250601083018_0001_D_stabilized.mp4 ->
// DJI_20250601083018_0001_D.MP4), then the name before the first
// underscore (GX010123_wide_stabilized.mp4 -> GX010123.MP4), with any case
// of extension.
func FindOriginal(stabilizedPath string) (string, error) {
	dir := filepath.Dir(stabilizedPath)
	base := filepath.Base(stabilizedPath)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	var stems []string
	if i := strings.Index(strings.ToLower(stem), "_stabilized"); i > 0 {
		stems = append(stems, stem[:i])
	}
	if i := strings.Index(stem, "_"); i > 0 {
		stems = append(stems, stem[:i])
	}
	if len(stems) == 0 {
		stems = append(stems, stem)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	for _, st := range stems {
		for _, e := range entries {
			name := e.Name()
			if name != base && strings.EqualFold(name, st+".MP4") {
				return filepath.Join(dir, name), nil
			}
		}
	}
	return "", originalMissing{stems[0] + ".MP4"}
}

// Trim is where a stabilized export starts inside the original.
type Trim struct {
	Start       float64 // seconds into the original
	Uncertainty float64 // seconds
}

// EstimateTrim uses the difference in creation times, which Gyroflow sets to
// the export's start. Creation times have one-second precision and the match
// was within a second or two on real clips, hence the uncertainty.
func EstimateTrim(stabilized, original *mp4.File) Trim {
	start := stabilized.CreationTime.Sub(original.CreationTime).Seconds()
	start = max(0, min(start, original.Duration))
	return Trim{Start: start, Uncertainty: 2}
}
