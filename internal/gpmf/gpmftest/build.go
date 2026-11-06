// Package gpmftest builds synthetic GPMF payloads for tests.
package gpmftest

import (
	"encoding/binary"
	"math"
)

// KLV encodes one GPMF entry, padding data to a multiple of 4 bytes.
func KLV(key string, typ byte, size uint8, repeat uint16, data []byte) []byte {
	b := append([]byte(key), typ, size)
	b = binary.BigEndian.AppendUint16(b, repeat)
	b = append(b, data...)
	for len(b)%4 != 0 {
		b = append(b, 0)
	}
	return b
}

// Nest wraps children in a nested entry such as DEVC or STRM.
func Nest(key string, children ...[]byte) []byte {
	var data []byte
	for _, c := range children {
		data = append(data, c...)
	}
	return KLV(key, 0, 1, uint16(len(data)), data)
}

// Str encodes a char string entry such as DVNM or STNM.
func Str(key, s string) []byte {
	return KLV(key, 'c', uint8(len(s)), 1, []byte(s))
}

// Int32s encodes rows of signed 32-bit values (type 'l').
func Int32s(key string, rows ...[]int32) []byte {
	var data []byte
	for _, r := range rows {
		for _, v := range r {
			data = binary.BigEndian.AppendUint32(data, uint32(v))
		}
	}
	return KLV(key, 'l', uint8(4*len(rows[0])), uint16(len(rows)), data)
}

// Uint32s encodes rows of unsigned 32-bit values (type 'L').
func Uint32s(key string, rows ...[]uint32) []byte {
	var data []byte
	for _, r := range rows {
		for _, v := range r {
			data = binary.BigEndian.AppendUint32(data, v)
		}
	}
	return KLV(key, 'L', uint8(4*len(rows[0])), uint16(len(rows)), data)
}

// Int16s encodes rows of signed 16-bit values (type 's').
func Int16s(key string, rows ...[]int16) []byte {
	var data []byte
	for _, r := range rows {
		for _, v := range r {
			data = binary.BigEndian.AppendUint16(data, uint16(v))
		}
	}
	return KLV(key, 's', uint8(2*len(rows[0])), uint16(len(rows)), data)
}

// Floats encodes rows of 32-bit floats (type 'f').
func Floats(key string, rows ...[]float32) []byte {
	var data []byte
	for _, r := range rows {
		for _, v := range r {
			data = binary.BigEndian.AppendUint32(data, math.Float32bits(v))
		}
	}
	return KLV(key, 'f', uint8(4*len(rows[0])), uint16(len(rows)), data)
}

// HERO10Second builds one second of HERO10-like telemetry: 10 GPS rows with
// the given fix and latitude, and 200 accelerometer rows reading 1 g.
func HERO10Second(fix uint32, lat float64) []byte {
	gps := make([][]int32, 10)
	for i := range gps {
		if fix >= 2 {
			gps[i] = []int32{int32(lat * 1e7), -760123456, 50000, 2150, 2200}
		} else {
			gps[i] = []int32{0, 0, 0, 0, 0}
		}
	}
	accl := make([][]int16, 200)
	for i := range accl {
		accl[i] = []int16{0, 0, 981}
	}
	return Nest("DEVC",
		Str("DVNM", "HERO10 Black"),
		Nest("STRM",
			Str("STNM", "GPS"),
			Uint32s("GPSF", []uint32{fix}),
			Int32s("SCAL", []int32{1e7, 1e7, 1000, 1000, 100}),
			Int32s("GPS5", gps...),
		),
		Nest("STRM",
			Str("ORIN", "ZXY"),
			Int16s("SCAL", []int16{100}),
			Int16s("ACCL", accl...),
		),
	)
}

// GPS9Rows encodes rows as the complex GPS9 stream newer GoPros write: type
// '?' described by TYPE "lllllllSS" (seven int32 then two uint16).
func GPS9Rows(rows ...[9]int64) []byte {
	var data []byte
	for _, r := range rows {
		for i, v := range r {
			if i < 7 {
				data = binary.BigEndian.AppendUint32(data, uint32(int32(v)))
			} else {
				data = binary.BigEndian.AppendUint16(data, uint16(v))
			}
		}
	}
	return append(Str("TYPE", "lllllllSS"), KLV("GPS9", '?', 32, uint16(len(rows)), data)...)
}
