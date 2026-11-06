package hud

import "math"

// SampleFlight makes up a flight for previewing designs: speed swells and
// falls, a climb, two flips, moving sticks, a draining battery and a loop
// path. It returns per-frame values and the path's points.
func SampleFlight(seconds, fps float64) (Values, [][2]float64) {
	n := int(seconds * fps)
	v := Values{}
	for _, p := range append(append([]Piece{}, Pieces...), dataSeries...) {
		v[p] = Series{V: make([]float64, n), OK: make([]bool, n)}
	}
	set := func(p Piece, i int, x float64) {
		v[p].V[i], v[p].OK[i] = x, true
	}
	for i := range n {
		t := float64(i) / fps
		loop := 2 * math.Pi * t / seconds
		set(Speed, i, 35+30*math.Sin(t/3))
		set(Height, i, 3+12*(1-math.Cos(loop))/2)
		set(GForce, i, 1+0.6*math.Abs(math.Sin(t/1.3)))
		set(Horizon, i, 25*math.Sin(t/2))
		set(Pitch, i, 12*math.Sin(t/2.5))
		set(StickThrottle, i, math.Sin(t/1.7))
		set(StickYaw, i, 0.4*math.Sin(t/2.3))
		set(StickPitch, i, 0.7*math.Sin(t/1.1))
		set(StickRoll, i, 0.6*math.Cos(t/1.4))
		v[Sticks].OK[i] = true
		set(Battery, i, 95-30*t/seconds)
		set(BatteryVolts, i, 16.4-1.8*t/seconds)
		set(PathX, i, 0.45*math.Sin(loop))
		set(PathY, i, 0.45*math.Sin(2*loop)/2)
		set(PathProgress, i, t/seconds)
		v[Path].OK[i] = true
		age := flipAgeCap
		for _, f := range []float64{seconds / 3, 2 * seconds / 3} {
			if t >= f {
				age = t - f
			}
			if t >= f && t < f+0.6 {
				v[Flip].V[i] = 1
			}
		}
		v[Flip].OK[i] = true
		set(FlipAge, i, age)
	}
	var pts [][2]float64
	for k := 0; k <= 100; k++ {
		loop := 2 * math.Pi * float64(k) / 100
		pts = append(pts, [2]float64{0.45 * math.Sin(loop), 0.45 * math.Sin(2*loop) / 2})
	}
	return v, pts
}
