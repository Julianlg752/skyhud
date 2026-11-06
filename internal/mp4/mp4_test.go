package mp4

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Julianlg752/skyhud/internal/mp4/mp4test"
)

var created = time.Date(2024, 5, 4, 14, 0, 0, 0, time.UTC)

func threeSamples(opt mp4test.Options) mp4test.Options {
	opt.CreationTime = created
	opt.Format = "gpmd"
	opt.Timescale = 1000
	opt.SampleDurations = []uint32{1001, 1001, 1001}
	opt.Payloads = [][]byte{[]byte("aaa"), []byte("bbbb"), []byte("cc")}
	return opt
}

func TestOpenReadsTrackAndSamples(t *testing.T) {
	f, err := Open(mp4test.Build(t, threeSamples(mp4test.Options{})))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if len(f.Tracks) != 1 || f.Tracks[0].Format != "gpmd" || f.Tracks[0].HandlerType != "meta" {
		t.Fatalf("tracks = %+v", f.Tracks)
	}
	s := f.Tracks[0].Samples
	if len(s) != 3 || math.Abs(s[1].Start-1.001) > 1e-9 || math.Abs(s[1].Duration-1.001) > 1e-9 {
		t.Fatalf("samples = %+v", s)
	}
	got, err := f.ReadSample(s[1])
	if err != nil || string(got) != "bbbb" {
		t.Fatalf("ReadSample = %q, %v", got, err)
	}
	if !f.CreationTime.Equal(created) {
		t.Fatalf("CreationTime = %v", f.CreationTime)
	}
	if math.Abs(f.Duration-3.003) > 1e-9 {
		t.Fatalf("Duration = %v", f.Duration)
	}
	if tr, ok := f.TrackByFormat("gpmd"); !ok || tr.ID != 1 {
		t.Fatalf("TrackByFormat = %+v, %v", tr, ok)
	}
}

func TestOpenMultipleSamplesPerChunk(t *testing.T) {
	f, err := Open(mp4test.Build(t, threeSamples(mp4test.Options{SamplesPerChunk: 2})))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for i, want := range []string{"aaa", "bbbb", "cc"} {
		got, err := f.ReadSample(f.Tracks[0].Samples[i])
		if err != nil || string(got) != want {
			t.Fatalf("sample %d = %q, %v", i, got, err)
		}
	}
}

func TestOpen64BitOffsets(t *testing.T) {
	f, err := Open(mp4test.Build(t, threeSamples(mp4test.Options{Use64BitOffsets: true})))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	got, err := f.ReadSample(f.Tracks[0].Samples[2])
	if err != nil || string(got) != "cc" {
		t.Fatalf("ReadSample = %q, %v", got, err)
	}
}

func TestOpenTruncated(t *testing.T) {
	f, err := Open(mp4test.Build(t, threeSamples(mp4test.Options{TruncateBytes: 3})))
	if err == nil {
		defer f.Close()
		_, err = f.ReadSample(f.Tracks[0].Samples[2])
	}
	if !errors.Is(err, ErrIncomplete) {
		t.Fatalf("err = %v, want ErrIncomplete", err)
	}
}

func TestOpenNotMP4(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.txt")
	os.WriteFile(path, []byte("these are not the bytes you are looking for"), 0o644)
	if f, err := Open(path); err == nil {
		f.Close()
		t.Fatal("expected an error")
	}
}

func TestRealGoProClip(t *testing.T) {
	path := os.Getenv("SKYHUD_GOPRO")
	if path == "" {
		t.Skip("set SKYHUD_GOPRO to a real GoPro clip")
	}
	f, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	tr, ok := f.TrackByFormat("gpmd")
	if !ok {
		t.Fatal("no gpmd track")
	}
	if len(tr.Samples) != 305 {
		t.Fatalf("gpmd track has %d samples, want 305", len(tr.Samples))
	}
	if math.Abs(f.Duration-304.87) > 0.1 {
		t.Fatalf("Duration = %v", f.Duration)
	}
}
