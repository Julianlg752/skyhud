package dji

import (
	"encoding/binary"
	"errors"
	"math"
)

// A minimal reader for the protobuf wire format: enough to walk DJI's
// undocumented djmd messages by field number.

var errProto = errors.New("dji: damaged data packet")

type field struct {
	num  uint64
	wire uint64
	v    uint64 // varint, fixed32 or fixed64 bits
	b    []byte // length-delimited payload
}

func parse(b []byte) ([]field, error) {
	var out []field
	for len(b) > 0 {
		key, n := binary.Uvarint(b)
		if n <= 0 {
			return nil, errProto
		}
		b = b[n:]
		f := field{num: key >> 3, wire: key & 7}
		switch f.wire {
		case 0:
			v, n := binary.Uvarint(b)
			if n <= 0 {
				return nil, errProto
			}
			f.v, b = v, b[n:]
		case 1:
			if len(b) < 8 {
				return nil, errProto
			}
			f.v, b = binary.LittleEndian.Uint64(b), b[8:]
		case 5:
			if len(b) < 4 {
				return nil, errProto
			}
			f.v, b = uint64(binary.LittleEndian.Uint32(b)), b[4:]
		case 2:
			l, n := binary.Uvarint(b)
			if n <= 0 || l > uint64(len(b)-n) {
				return nil, errProto
			}
			f.b, b = b[n:n+int(l)], b[n+int(l):]
		default:
			return nil, errProto
		}
		out = append(out, f)
	}
	return out, nil
}

// all returns the payloads of every message reached by following path.
func all(b []byte, path ...uint64) [][]byte {
	level := [][]byte{b}
	for _, num := range path {
		var next [][]byte
		for _, m := range level {
			fs, err := parse(m)
			if err != nil {
				continue
			}
			for _, f := range fs {
				if f.num == num && f.wire == 2 {
					next = append(next, f.b)
				}
			}
		}
		level = next
	}
	return level
}

// scalars returns the fixed or varint fields of message m by number.
func scalars(m []byte) map[uint64]field {
	out := map[uint64]field{}
	fs, _ := parse(m)
	for _, f := range fs {
		if f.wire != 2 {
			out[f.num] = f
		}
	}
	return out
}

func (f field) f32() float64 { return float64(math.Float32frombits(uint32(f.v))) }
func (f field) f64() float64 { return math.Float64frombits(f.v) }

// float32Field reads float field num of a message's scalars. protobuf leaves
// zero values out, so an absent field reads 0; a field of another wire type
// means the layout changed and ok is false.
func float32Field(s map[uint64]field, num uint64) (v float64, present, ok bool) {
	f, found := s[num]
	if !found {
		return 0, false, true
	}
	if f.wire != 5 {
		return 0, true, false
	}
	return f.f32(), true, true
}
