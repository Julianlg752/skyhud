// Package djitest builds synthetic Avata 2 djmd packets for tests, shaped
// like the real ones (see internal/dji for the field map).
package djitest

import (
	"encoding/binary"
	"math"
)

// Frame is the content of one packet.
type Frame struct {
	Quats    [][4]float32 // w, x, y, z
	Vel      [3]float32   // north, east, down, m/s
	LatLon   [2]float64   // radians
	HeightMM float32
	Device   string
	Serial   string // written to .1.1.5 like a real camera serial
	NoHeight bool   // leave the height field out
	// Layout changes, as a firmware update might bring:
	VelField     int  // first velocity field number (default 1)
	HeightVarint bool // write height as a varint instead of a float
}

func varint(v uint64) []byte { return binary.AppendUvarint(nil, v) }

func tag(num, wire int) []byte { return varint(uint64(num)<<3 | uint64(wire)) }

// Msg encodes a length-delimited field.
func Msg(num int, parts ...[]byte) []byte {
	var body []byte
	for _, p := range parts {
		body = append(body, p...)
	}
	return append(append(tag(num, 2), varint(uint64(len(body)))...), body...)
}

// F32 encodes a fixed32 float field.
func F32(num int, v float32) []byte {
	return binary.LittleEndian.AppendUint32(tag(num, 5), math.Float32bits(v))
}

// F64 encodes a fixed64 float field.
func F64(num int, v float64) []byte {
	return binary.LittleEndian.AppendUint64(tag(num, 1), math.Float64bits(v))
}

// Uint encodes a varint field.
func Uint(num int, v uint64) []byte { return append(tag(num, 0), varint(v)...) }

// Str encodes a string field.
func Str(num int, s string) []byte { return Msg(num, []byte(s)) }

// Packet encodes one frame.
func Packet(f Frame) []byte {
	var head [][]byte
	if f.Device != "" {
		head = append(head, Str(10, f.Device))
	}
	if f.Serial != "" {
		head = append(head, Str(5, f.Serial))
	}
	var quats [][]byte
	quats = append(quats, Uint(1, 128091891))
	for _, q := range f.Quats {
		quats = append(quats, Msg(3, F32(1, q[0]), F32(2, q[1]), F32(3, q[2]), F32(4, q[3])))
	}
	vf := f.VelField
	if vf == 0 {
		vf = 1
	}
	flight := [][]byte{
		Msg(2, F32(vf, f.Vel[0]), F32(vf+1, f.Vel[1]), F32(vf+2, f.Vel[2])),
		Msg(4, Msg(1, F64(2, f.LatLon[0]), F64(3, f.LatLon[1]))),
	}
	if !f.NoHeight {
		var h []byte
		if f.HeightMM != 0 { // protobuf leaves zero values out
			h = F32(1, f.HeightMM)
			if f.HeightVarint {
				h = Uint(1, uint64(f.HeightMM))
			}
		}
		flight = append(flight, Msg(5, h, Uint(2, 1)))
	}
	return append(Msg(1, Msg(1, head...)), Msg(3, Msg(3, Msg(2, quats...)), Msg(4, flight...))...)
}
