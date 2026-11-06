// Package gpmf decodes GoPro's GPMF telemetry format: nested key-length-value
// entries, with SCAL dividing the numeric data that follows it in a stream.
package gpmf

import (
	"encoding/binary"
	"errors"
	"math"
	"strings"
)

// ErrMalformed means an entry's size runs past the end of its container.
var ErrMalformed = errors.New("gpmf: malformed data")

// Stream is one numeric telemetry stream, such as GPS5 or ACCL.
type Stream struct {
	Key   string   // e.g. "GPS5"
	Name  string   // STNM
	Units []string // SIUN or UNIT, may be empty
	Orin  string   // ORIN axis order, may be empty
	Rows  [][]float64
}

// Payload is one decoded GPMF sample.
type Payload struct {
	Device  string // DVNM
	Streams []Stream
}

// Keys that describe a stream rather than carrying its data.
var metaKeys = map[string]bool{
	"DEVC": true, "DVID": true, "DVNM": true, "STRM": true, "STNM": true,
	"SIUN": true, "UNIT": true, "SCAL": true, "TSMP": true, "STMP": true,
	"TICK": true, "TOCK": true, "TYPE": true, "EMPT": true, "ORIN": true,
	"ORIO": true, "MTRX": true, "TMPC": true, "RMRK": true, "TIMO": true,
}

var typeWidth = map[byte]int{
	'b': 1, 'B': 1, 's': 2, 'S': 2, 'l': 4, 'L': 4, 'f': 4, 'd': 8,
	'j': 8, 'J': 8, 'q': 4, 'Q': 8,
}

// Decode parses one GPMF payload.
func Decode(b []byte) (Payload, error) {
	var p Payload
	err := walk(b, &p, nil)
	return p, err
}

// streamState carries the descriptors seen so far inside one STRM.
type streamState struct {
	name  string
	units []string
	orin  string
	scale []float64
	typ   string // TYPE: field types of a complex ('?') stream
}

func walk(b []byte, p *Payload, st *streamState) error {
	for len(b) > 0 {
		if len(b) < 8 {
			return ErrMalformed
		}
		key := string(b[:4])
		typ := b[4]
		size := int(b[5])
		repeat := int(binary.BigEndian.Uint16(b[6:8]))
		n := size * repeat
		padded := (n + 3) &^ 3
		if 8+padded > len(b) {
			return ErrMalformed
		}
		data := b[8 : 8+n]
		b = b[8+padded:]

		if typ == 0 {
			child := st
			if key == "STRM" {
				child = &streamState{}
			}
			if err := walk(data, p, child); err != nil {
				return err
			}
			continue
		}
		switch key {
		case "DVNM":
			p.Device = cString(data)
			continue
		}
		if st == nil {
			continue
		}
		switch key {
		case "STNM":
			st.name = cString(data)
		case "ORIN":
			st.orin = cString(data)
		case "SIUN", "UNIT":
			st.units = splitStrings(data, size)
		case "SCAL":
			st.scale = values(data, typ)
		case "TYPE":
			st.typ = cString(data)
		default:
			if metaKeys[key] {
				continue
			}
			if typ == '?' {
				if rows := complexRows(data, size, repeat, st.typ); rows != nil {
					for r := range rows {
						rows[r] = scaleRow(rows[r], st.scale)
					}
					p.Streams = append(p.Streams, Stream{Key: key, Name: st.name, Units: st.units, Orin: st.orin, Rows: rows})
				}
				continue
			}
			w, ok := typeWidth[typ]
			if !ok || size == 0 || size%w != 0 {
				continue // chars, complex types: not numeric telemetry
			}
			cols := size / w
			vals := values(data, typ)
			rows := make([][]float64, repeat)
			for r := range rows {
				rows[r] = scaleRow(vals[r*cols:(r+1)*cols], st.scale)
			}
			p.Streams = append(p.Streams, Stream{Key: key, Name: st.name, Units: st.units, Orin: st.orin, Rows: rows})
		}
	}
	return nil
}

// complexRows splits a complex stream into rows using its TYPE string, one
// character per field (e.g. "lllllllSS" for GPS9). It returns nil when the
// TYPE doesn't describe the row size.
func complexRows(data []byte, size, repeat int, types string) [][]float64 {
	if types == "" {
		return nil
	}
	rowSize := 0
	for i := 0; i < len(types); i++ {
		w, ok := typeWidth[types[i]]
		if !ok {
			return nil
		}
		rowSize += w
	}
	if rowSize != size {
		return nil
	}
	rows := make([][]float64, repeat)
	for r := range rows {
		off := r * size
		row := make([]float64, 0, len(types))
		for i := 0; i < len(types); i++ {
			w := typeWidth[types[i]]
			row = append(row, values(data[off:off+w], types[i])...)
			off += w
		}
		rows[r] = row
	}
	return rows
}

// scaleRow divides by SCAL when it has one value or one per column.
func scaleRow(row, scale []float64) []float64 {
	switch {
	case len(scale) == 1 && scale[0] != 0:
		for i := range row {
			row[i] /= scale[0]
		}
	case len(scale) == len(row):
		for i := range row {
			if scale[i] != 0 {
				row[i] /= scale[i]
			}
		}
	}
	return row
}

func values(data []byte, typ byte) []float64 {
	w := typeWidth[typ]
	if w == 0 {
		return nil
	}
	out := make([]float64, 0, len(data)/w)
	for i := 0; i+w <= len(data); i += w {
		d := data[i : i+w]
		var v float64
		switch typ {
		case 'b':
			v = float64(int8(d[0]))
		case 'B':
			v = float64(d[0])
		case 's':
			v = float64(int16(binary.BigEndian.Uint16(d)))
		case 'S':
			v = float64(binary.BigEndian.Uint16(d))
		case 'l':
			v = float64(int32(binary.BigEndian.Uint32(d)))
		case 'L':
			v = float64(binary.BigEndian.Uint32(d))
		case 'f':
			v = float64(math.Float32frombits(binary.BigEndian.Uint32(d)))
		case 'd':
			v = math.Float64frombits(binary.BigEndian.Uint64(d))
		case 'j':
			v = float64(int64(binary.BigEndian.Uint64(d)))
		case 'J':
			v = float64(binary.BigEndian.Uint64(d))
		case 'q':
			v = float64(int32(binary.BigEndian.Uint32(d))) / 65536
		case 'Q':
			v = float64(int64(binary.BigEndian.Uint64(d))) / 4294967296
		}
		out = append(out, v)
	}
	return out
}

func cString(b []byte) string {
	return strings.TrimRight(string(b), "\x00 ")
}

func splitStrings(data []byte, size int) []string {
	if size == 0 {
		return nil
	}
	var out []string
	for i := 0; i+size <= len(data); i += size {
		out = append(out, cString(data[i:i+size]))
	}
	return out
}
