package gpmf

import (
	"math"
	"math/rand"
	"testing"

	g "github.com/Julianlg752/skyhud/internal/gpmf/gpmftest"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func stream(t *testing.T, p Payload, key string) Stream {
	t.Helper()
	for _, s := range p.Streams {
		if s.Key == key {
			return s
		}
	}
	t.Fatalf("no %s stream in %+v", key, p.Streams)
	return Stream{}
}

func TestDecodeScaledStream(t *testing.T) {
	b := g.Nest("DEVC",
		g.Str("DVNM", "HERO10 Black"),
		g.Nest("STRM",
			g.Str("STNM", "GPS"),
			g.Int32s("SCAL", []int32{1e7, 1e7, 1000, 1000, 100}),
			g.Int32s("GPS5", []int32{401234567, -760123456, 50000, 2150, 2200}),
		),
	)
	p, err := Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	if p.Device != "HERO10 Black" {
		t.Fatalf("Device = %q", p.Device)
	}
	s := stream(t, p, "GPS5")
	want := []float64{40.1234567, -76.0123456, 50, 2.15, 22}
	if s.Name != "GPS" || len(s.Rows) != 1 {
		t.Fatalf("stream = %+v", s)
	}
	for i, w := range want {
		if !near(s.Rows[0][i], w) {
			t.Fatalf("row = %v, want %v", s.Rows[0], want)
		}
	}
}

func TestDecodeSingleScale(t *testing.T) {
	b := g.Nest("DEVC", g.Nest("STRM",
		g.Int16s("SCAL", []int16{100}),
		g.Int16s("ACCL", []int16{980, 0, -50}),
	))
	p, err := Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	r := stream(t, p, "ACCL").Rows[0]
	if !near(r[0], 9.8) || !near(r[1], 0) || !near(r[2], -0.5) {
		t.Fatalf("row = %v", r)
	}
}

func TestDecodeScaleCountMismatchLeavesValuesUnscaled(t *testing.T) {
	b := g.Nest("DEVC", g.Nest("STRM",
		g.Int32s("SCAL", []int32{1e7, 1e7, 1000, 1000, 100}),
		g.Uint32s("GPSF", []uint32{3}),
	))
	p, err := Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	if r := stream(t, p, "GPSF").Rows[0]; r[0] != 3 {
		t.Fatalf("GPSF = %v, want 3", r)
	}
}

func TestDecodePadding(t *testing.T) {
	b := g.Nest("DEVC", g.Nest("STRM",
		g.Str("STNM", "Gyros"), // 5 chars, padded to 8
		g.Floats("GYRO", []float32{1, 2, 3}),
	))
	p, err := Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	s := stream(t, p, "GYRO")
	if s.Name != "Gyros" || s.Rows[0][2] != 3 {
		t.Fatalf("stream = %+v", s)
	}
}

func TestDecodeOrientationString(t *testing.T) {
	b := g.Nest("DEVC", g.Nest("STRM",
		g.Str("ORIN", "ZXY"),
		g.Int16s("ACCL", []int16{1, 2, 3}),
	))
	p, err := Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	if s := stream(t, p, "ACCL"); s.Orin != "ZXY" {
		t.Fatalf("Orin = %q", s.Orin)
	}
}

func TestDecodeGarbage(t *testing.T) {
	overrun := g.KLV("ACCL", 's', 6, 1000, []byte{1, 2, 3, 4})
	if _, err := Decode(overrun); err == nil {
		t.Fatal("expected an error for a size that overruns the buffer")
	}
	r := rand.New(rand.NewSource(1))
	for i := 0; i < 2000; i++ {
		b := make([]byte, r.Intn(64))
		r.Read(b)
		Decode(b) // must not panic
	}
}

func TestDecodeComplexTypeStream(t *testing.T) {
	b := g.Nest("DEVC", g.Nest("STRM",
		g.Int32s("SCAL", []int32{1e7, 1e7, 1000, 1000, 100, 1, 1000, 100, 1}),
		g.GPS9Rows([9]int64{401000000, -760123456, 50000, 3000, 3100, 9000, 45000000, 120, 3}),
	))
	p, err := Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	r := stream(t, p, "GPS9").Rows[0]
	want := []float64{40.1, -76.0123456, 50, 3, 31, 9000, 45000, 1.2, 3}
	for i, w := range want {
		if !near(r[i], w) {
			t.Fatalf("row = %v, want %v", r, want)
		}
	}
}
