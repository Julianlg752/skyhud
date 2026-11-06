package gyroflow

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Julianlg752/skyhud/internal/mp4"
	"github.com/Julianlg752/skyhud/internal/mp4/mp4test"
)

func touch(t *testing.T, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestFindOriginal(t *testing.T) {
	dir := t.TempDir()
	orig := touch(t, dir, "GX010123.MP4")
	for _, name := range []string{"GX010123_stabilized.mp4", "GX010123_stabilized_1.mp4", "GX010123_wide_stabilized.mp4"} {
		got, err := FindOriginal(touch(t, dir, name))
		if err != nil || got != orig {
			t.Fatalf("%s: got %q, %v", name, got, err)
		}
		if !IsStabilized(filepath.Join(dir, name)) {
			t.Fatalf("%s should count as stabilized", name)
		}
	}
	if IsStabilized(orig) {
		t.Fatal("the original is not stabilized")
	}
}

func TestFindOriginalMissing(t *testing.T) {
	_, err := FindOriginal(touch(t, t.TempDir(), "GX010123_stabilized.mp4"))
	if !errors.Is(err, ErrOriginalMissing) {
		t.Fatalf("err = %v", err)
	}
	want := "the original camera file GX010123.MP4 was not found next to the stabilized clip; it holds the flight data"
	if err.Error() != want {
		t.Fatalf("message = %q", err.Error())
	}
}

func open(t *testing.T, name string, created time.Time, durMs uint32) *mp4.File {
	t.Helper()
	f, err := mp4.Open(mp4test.Build(t, mp4test.Options{Name: name, CreationTime: created, Format: "hvc1",
		Timescale: 1000, SampleDurations: []uint32{durMs}, Payloads: [][]byte{[]byte("f")}}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

func TestEstimateTrim(t *testing.T) {
	origTime := time.Date(2024, 5, 4, 14, 0, 0, 0, time.UTC)
	orig := open(t, "GX010123.MP4", origTime, 304870)
	stab := open(t, "GX010123_stabilized.mp4", time.Date(2024, 5, 4, 14, 1, 36, 0, time.UTC), 66700)
	if got := EstimateTrim(stab, orig); got.Start != 96 || got.Uncertainty != 2 {
		t.Fatalf("trim = %+v", got)
	}
	early := open(t, "GX010123_stabilized_1.mp4", origTime.Add(-time.Minute), 66700)
	if got := EstimateTrim(early, orig); got.Start != 0 {
		t.Fatalf("trim before the original = %+v", got)
	}
}

func TestRealGyroflowClip(t *testing.T) {
	clip := os.Getenv("SKYHUD_GOPRO")
	if clip == "" {
		t.Skip("set SKYHUD_GOPRO to a GoPro clip with its Gyroflow export <name>_stabilized.mp4 next to it")
	}
	stabPath := strings.TrimSuffix(clip, filepath.Ext(clip)) + "_stabilized.mp4"
	if _, err := os.Stat(stabPath); err != nil {
		t.Skip(err)
	}
	origPath, err := FindOriginal(stabPath)
	if err != nil || filepath.Base(origPath) != filepath.Base(clip) {
		t.Fatalf("FindOriginal = %q, %v", origPath, err)
	}
	stab, err := mp4.Open(stabPath)
	if err != nil {
		t.Fatal(err)
	}
	defer stab.Close()
	orig, err := mp4.Open(origPath)
	if err != nil {
		t.Fatal(err)
	}
	defer orig.Close()
	got := EstimateTrim(stab, orig)
	// SKYHUD_GOPRO_TRIM: the expected trim start in seconds, if known.
	if want, err := strconv.ParseFloat(os.Getenv("SKYHUD_GOPRO_TRIM"), 64); err == nil && math.Abs(got.Start-want) > 0.5 {
		t.Fatalf("trim = %+v, want start %v", got, want)
	}
	t.Logf("trim = %+v", got)
}

func TestFindOriginalDJI(t *testing.T) {
	dir := t.TempDir()
	orig := touch(t, dir, "DJI_20250601083018_0001_D.MP4")
	touch(t, dir, "DJI_20250601090000_0002_D.MP4")
	for _, name := range []string{"DJI_20250601083018_0001_D_stabilized.mp4", "DJI_20250601083018_0001_D_stabilized_1.mp4"} {
		got, err := FindOriginal(touch(t, dir, name))
		if err != nil || got != orig {
			t.Fatalf("%s: got %q, %v", name, got, err)
		}
	}
	_, err := FindOriginal(touch(t, t.TempDir(), "DJI_20250601083018_0001_D_stabilized.mp4"))
	if !errors.Is(err, ErrOriginalMissing) || !strings.Contains(err.Error(), "DJI_20250601083018_0001_D.MP4") || !strings.Contains(err.Error(), "original camera file") {
		t.Fatalf("err = %v", err)
	}
}
