// Package djilogtest writes synthetic DJI Fly flight logs for tests: the
// encoder side of internal/djilog's reader (XOR scrambling only; encrypted
// records are left to the djilog package's own tests).
package djilogtest

import (
	"bytes"
	"encoding/binary"
	"time"
)

// Frame is one OSD record and the records written after it.
type Frame struct {
	At                                  time.Time
	Height                              float64 // m
	Throttle, Rudder, Elevator, Aileron uint16
	Battery                             byte
	Volts                               float64
}

// V12 builds a version 12 log (XOR only).
func V12(aircraft string, start time.Time, frames []Frame) []byte {
	var f bytes.Buffer
	pre := make([]byte, 100)
	pre[10] = 12
	f.Write(pre)
	f.Write(details(aircraft, start))
	for _, fr := range frames {
		record(&f, 12, 1, osd(fr.Height))
		record(&f, 12, 5, custom(fr.At))
		record(&f, 12, 4, rc(fr.Aileron, fr.Elevator, fr.Throttle, fr.Rudder))
		record(&f, 12, 7, battery(fr.Battery, fr.Volts))
	}
	return f.Bytes()
}

// V14Header builds a version 14 log with details but no records: enough to
// need keys from DJI's service.
func V14Header(aircraft string, start time.Time) []byte {
	var f bytes.Buffer
	pre := make([]byte, 100)
	pre[10] = 14
	d := details(aircraft, start)
	info := append([]byte{1}, binary.LittleEndian.AppendUint16(nil, uint16(len(d)))...)
	info = append(info, d...)
	info = append(info, 0, 0) // no signature
	payload := scramble(0, info)
	aux0 := append([]byte{0}, binary.LittleEndian.AppendUint16(nil, uint16(len(payload)))...)
	aux0 = append(aux0, payload...)
	aux1 := []byte{1, 3, 0, 1, 0, 3} // version 1, department 3 (DJI Fly)
	binary.LittleEndian.PutUint64(pre, uint64(100+len(aux0)+len(aux1)))
	f.Write(pre)
	f.Write(aux0)
	f.Write(aux1)
	return f.Bytes()
}

// Duration is the flight time written into the details (total_time).
var Duration = 90 * time.Second

func details(aircraft string, start time.Time) []byte {
	d := make([]byte, 436)
	binary.LittleEndian.PutUint64(d[91:], uint64(start.UnixMilli()))
	binary.LittleEndian.PutUint32(d[119:], uint32(Duration.Milliseconds()))
	d[271] = 152
	copy(d[280:312], aircraft)
	return d
}

func record(f *bytes.Buffer, version, typ byte, content []byte) {
	payload := scramble(typ, content)
	f.WriteByte(typ)
	if version <= 12 {
		f.WriteByte(byte(len(payload)))
	} else {
		f.Write(binary.LittleEndian.AppendUint16(nil, uint16(len(payload))))
	}
	f.Write(payload)
	f.WriteByte(0xFF)
}

// scramble prepends a seed byte and XORs content with the key DJI derives
// from it (CRC-64 Jones, reflected, as in dji-log-parser).
func scramble(typ byte, content []byte) []byte {
	var seed byte = 0x5a
	magic := uint64(0x123456789ABCDEF0)
	var data [8]byte
	binary.LittleEndian.PutUint64(data[:], magic*uint64(seed))
	crc := uint64(seed + typ)
	for _, b := range data {
		crc = table[byte(crc)^b] ^ (crc >> 8)
	}
	var key [8]byte
	binary.LittleEndian.PutUint64(key[:], crc)
	out := []byte{seed}
	for i, b := range content {
		out = append(out, b^key[i%8])
	}
	return out
}

var table = func() (t [256]uint64) {
	for i := range t {
		c := uint64(i)
		for range 8 {
			if c&1 == 1 {
				c = c>>1 ^ 0x95AC9329AC4BC9B5
			} else {
				c >>= 1
			}
		}
		t[i] = c
	}
	return
}()

func osd(height float64) []byte {
	b := make([]byte, 40)
	binary.LittleEndian.PutUint16(b[16:], uint16(int16(height*10)))
	return b
}

func custom(at time.Time) []byte {
	b := make([]byte, 18)
	binary.LittleEndian.PutUint64(b[10:], uint64(at.UnixMilli()))
	return b
}

func rc(aileron, elevator, throttle, rudder uint16) []byte {
	b := make([]byte, 13)
	for i, v := range []uint16{aileron, elevator, throttle, rudder, 1024} {
		binary.LittleEndian.PutUint16(b[2*i:], v)
	}
	return b
}

func battery(pct byte, volts float64) []byte {
	b := make([]byte, 40)
	b[0] = pct
	binary.LittleEndian.PutUint16(b[1:], uint16(volts*1000))
	return b
}
