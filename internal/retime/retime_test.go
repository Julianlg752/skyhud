package retime

import (
	"math"
	"testing"
)

const frame = 1001.0 / 30000

func near(t *testing.T, name string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Errorf("%s = %v, want %v", name, got, want)
	}
}

func TestConstant(t *testing.T) {
	near(t, "50%", Map{Start: 20, Speed: 0.5}.SourceAt(4), 22, 1e-9)
	freeze := Map{Start: 20, Speed: 0}
	for _, at := range []float64{0, 3, 100} {
		near(t, "freeze", freeze.SourceAt(at), 20, 1e-9)
	}
}

// frames builds keys from clip/source frame pairs.
func frames(pairs ...[2]float64) []Key {
	var keys []Key
	for _, p := range pairs {
		keys = append(keys, Key{Clip: p[0] * frame, Source: p[1] * frame})
	}
	return keys
}

// The case verified on rendered frames in Resolve.
func TestRampStartsMidFile(t *testing.T) {
	m := Map{Start: 200 * frame, Keys: frames([2]float64{0, 0}, [2]float64{100, 25}, [2]float64{300, 225}, [2]float64{500, 275}, [2]float64{1500, 1275})}
	if !m.Ramped() {
		t.Fatal("Ramped() = false")
	}
	for _, c := range [][2]float64{{0, 200}, {30, 226.25}, {200, 268.75}, {230, 280}, {390, 440}} {
		near(t, "source frame", m.SourceAt(c[0]*frame)/frame, c[1], 1e-6)
	}
}

func TestRampBeyondKeys(t *testing.T) {
	m := Map{Start: 5, Keys: []Key{{0, 0}, {10, 5}, {20, 25}}}
	// Start 5 is reached at clip time 10; 15 s later is clip 25, past the last key.
	near(t, "after last key", m.SourceAt(15), 35, 1e-9)
	before := Map{Start: -1, Keys: []Key{{0, 0}, {10, 5}}}
	// -1 is before the first key: extend the first segment (slope 0.5).
	near(t, "before first key", before.SourceAt(0), -1, 1e-9)
	near(t, "before first key, later", before.SourceAt(2), 0, 1e-9)
}

func TestRampFreezeAndReverse(t *testing.T) {
	keys := []Key{{0, 0}, {10, 10}, {20, 10}, {30, 0}}
	near(t, "start 5", Map{Start: 5, Keys: keys}.SourceAt(0), 5, 1e-9)
	near(t, "start 5 later", Map{Start: 5, Keys: keys}.SourceAt(3), 8, 1e-9)
	onFreeze := Map{Start: 10, Keys: keys}
	for _, at := range []float64{0, 5, 10} {
		near(t, "on freeze", onFreeze.SourceAt(at), 10, 1e-9)
	}
	near(t, "reverse after freeze", onFreeze.SourceAt(15), 5, 1e-9)
}

func TestSingleKeyIsNotARamp(t *testing.T) {
	m := Map{Start: 3, Speed: 1, Keys: []Key{{0, 0}}}
	if m.Ramped() {
		t.Fatal("one key counted as a ramp")
	}
	near(t, "falls back to speed", m.SourceAt(2), 5, 1e-9)
}
