// Package mp4 reads the parts of an MP4 file that telemetry needs: tracks,
// their sample tables, and the file's creation time. Sample data is read on
// demand, so multi-gigabyte files never load into memory.
package mp4

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"time"
)

// ErrIncomplete means the file ends early or its structure points past its end.
var ErrIncomplete = errors.New("mp4: file looks incomplete or damaged")

// ErrNotMP4 means the file doesn't start like an MP4.
var ErrNotMP4 = errors.New("mp4: not an MP4 file")

// File is an open MP4 file.
type File struct {
	CreationTime time.Time
	Duration     float64 // seconds
	Tracks       []Track

	f    *os.File
	size int64
}

// Track is one track and its samples.
type Track struct {
	ID          uint32
	HandlerType string // e.g. "vide", "soun", "meta"
	HandlerName string
	Format      string // sample entry fourcc, e.g. "gpmd", "djmd", "hvc1"
	Timescale   uint32
	Samples     []Sample
}

// Sample locates one sample in the file and in time.
type Sample struct {
	Offset          int64
	Size            uint32
	Start, Duration float64 // seconds
}

var mp4Epoch = time.Date(1904, 1, 1, 0, 0, 0, 0, time.UTC)

// Open reads the file's structure.
func Open(path string) (*File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	m := &File{f: f, size: st.Size()}
	if err := m.readTop(); err != nil {
		f.Close()
		return nil, err
	}
	return m, nil
}

// Close closes the underlying file.
func (m *File) Close() error { return m.f.Close() }

// TrackByFormat returns the first track whose sample entry has the given fourcc.
func (m *File) TrackByFormat(format string) (*Track, bool) {
	for i := range m.Tracks {
		if m.Tracks[i].Format == format {
			return &m.Tracks[i], true
		}
	}
	return nil, false
}

// ReadSample returns the bytes of one sample.
func (m *File) ReadSample(s Sample) ([]byte, error) {
	if s.Offset < 0 || s.Offset+int64(s.Size) > m.size {
		return nil, ErrIncomplete
	}
	b := make([]byte, s.Size)
	if _, err := m.f.ReadAt(b, s.Offset); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, ErrIncomplete
		}
		return nil, err
	}
	return b, nil
}

type boxHeader struct {
	typ        string
	start, end int64 // payload range in the file
}

// header reads a box header at off, bounded by limit.
func (m *File) header(off, limit int64) (boxHeader, error) {
	var h [16]byte
	if off+8 > limit {
		return boxHeader{}, ErrIncomplete
	}
	if _, err := m.f.ReadAt(h[:8], off); err != nil {
		return boxHeader{}, ErrIncomplete
	}
	size := int64(binary.BigEndian.Uint32(h[:4]))
	typ := string(h[4:8])
	start := off + 8
	switch size {
	case 0:
		size = limit - off
	case 1:
		if off+16 > limit {
			return boxHeader{}, ErrIncomplete
		}
		if _, err := m.f.ReadAt(h[8:16], off+8); err != nil {
			return boxHeader{}, ErrIncomplete
		}
		size = int64(binary.BigEndian.Uint64(h[8:16]))
		start = off + 16
	}
	if size < start-off || off+size > limit {
		return boxHeader{}, ErrIncomplete
	}
	return boxHeader{typ: typ, start: start, end: off + size}, nil
}

func (m *File) readTop() error {
	first, err := m.header(0, m.size)
	if err != nil || !isTopLevel(first.typ) {
		return ErrNotMP4
	}
	var moov []byte
	for off := int64(0); off < m.size; {
		h, err := m.header(off, m.size)
		if err != nil {
			return err
		}
		if h.typ == "moov" {
			moov = make([]byte, h.end-h.start)
			if _, err := m.f.ReadAt(moov, h.start); err != nil {
				return ErrIncomplete
			}
		}
		off = h.end
	}
	if moov == nil {
		return fmt.Errorf("%w: no movie header", ErrIncomplete)
	}
	return m.parseMoov(moov)
}

func isTopLevel(typ string) bool {
	switch typ {
	case "ftyp", "moov", "mdat", "free", "skip", "wide", "uuid":
		return true
	}
	return false
}

// children splits an in-memory container payload into its boxes.
func children(b []byte) (map[string][][]byte, error) {
	out := map[string][][]byte{}
	for len(b) > 0 {
		if len(b) < 8 {
			return nil, ErrIncomplete
		}
		size := uint64(binary.BigEndian.Uint32(b))
		typ := string(b[4:8])
		hdr := uint64(8)
		switch size {
		case 0:
			size = uint64(len(b))
		case 1:
			if len(b) < 16 {
				return nil, ErrIncomplete
			}
			size = binary.BigEndian.Uint64(b[8:])
			hdr = 16
		}
		if size < hdr || size > uint64(len(b)) {
			return nil, ErrIncomplete
		}
		out[typ] = append(out[typ], b[hdr:size])
		b = b[size:]
	}
	return out, nil
}

func one(boxes map[string][][]byte, typ string) ([]byte, error) {
	if len(boxes[typ]) == 0 {
		return nil, fmt.Errorf("%w: missing %s box", ErrIncomplete, typ)
	}
	return boxes[typ][0], nil
}

func (m *File) parseMoov(moov []byte) error {
	boxes, err := children(moov)
	if err != nil {
		return err
	}
	mvhd, err := one(boxes, "mvhd")
	if err != nil {
		return err
	}
	created, timescale, duration, err := timesAndScale(mvhd)
	if err != nil {
		return err
	}
	m.CreationTime = created
	if timescale > 0 {
		m.Duration = float64(duration) / float64(timescale)
	}
	for _, trak := range boxes["trak"] {
		t, err := m.parseTrak(trak)
		if err != nil {
			return err
		}
		m.Tracks = append(m.Tracks, t)
	}
	return nil
}

// timesAndScale reads creation time, timescale and duration from mvhd or mdhd.
func timesAndScale(b []byte) (time.Time, uint32, uint64, error) {
	if len(b) < 4 {
		return time.Time{}, 0, 0, ErrIncomplete
	}
	if b[0] == 1 {
		if len(b) < 32 {
			return time.Time{}, 0, 0, ErrIncomplete
		}
		c := binary.BigEndian.Uint64(b[4:])
		return mp4Epoch.Add(time.Duration(c) * time.Second), binary.BigEndian.Uint32(b[20:]), binary.BigEndian.Uint64(b[24:]), nil
	}
	if len(b) < 20 {
		return time.Time{}, 0, 0, ErrIncomplete
	}
	c := binary.BigEndian.Uint32(b[4:])
	return mp4Epoch.Add(time.Duration(c) * time.Second), binary.BigEndian.Uint32(b[12:]), uint64(binary.BigEndian.Uint32(b[16:])), nil
}

func (m *File) parseTrak(trak []byte) (Track, error) {
	var t Track
	boxes, err := children(trak)
	if err != nil {
		return t, err
	}
	if tkhd, err := one(boxes, "tkhd"); err == nil {
		if len(tkhd) >= 24 && tkhd[0] == 1 {
			t.ID = binary.BigEndian.Uint32(tkhd[20:])
		} else if len(tkhd) >= 16 {
			t.ID = binary.BigEndian.Uint32(tkhd[12:])
		}
	}
	mdiaB, err := one(boxes, "mdia")
	if err != nil {
		return t, err
	}
	mdia, err := children(mdiaB)
	if err != nil {
		return t, err
	}
	mdhd, err := one(mdia, "mdhd")
	if err != nil {
		return t, err
	}
	if _, t.Timescale, _, err = timesAndScale(mdhd); err != nil {
		return t, err
	}
	if hdlr, err := one(mdia, "hdlr"); err == nil && len(hdlr) >= 24 {
		t.HandlerType = string(hdlr[8:12])
		name := hdlr[24:]
		for i, c := range name {
			if c == 0 {
				name = name[:i]
				break
			}
		}
		t.HandlerName = string(name)
	}
	minfB, err := one(mdia, "minf")
	if err != nil {
		return t, err
	}
	minf, err := children(minfB)
	if err != nil {
		return t, err
	}
	stblB, err := one(minf, "stbl")
	if err != nil {
		return t, err
	}
	stbl, err := children(stblB)
	if err != nil {
		return t, err
	}
	if stsd, err := one(stbl, "stsd"); err == nil && len(stsd) >= 16 {
		t.Format = string(stsd[12:16])
	}
	t.Samples, err = m.sampleTable(stbl, t.Timescale)
	return t, err
}

// table reads count fixed-width entries that follow a version/flags word and a count.
func table(b []byte, width int) ([]byte, uint32, error) {
	if len(b) < 8 {
		return nil, 0, ErrIncomplete
	}
	n := binary.BigEndian.Uint32(b[4:])
	if uint64(n)*uint64(width) > uint64(len(b)-8) {
		return nil, 0, ErrIncomplete
	}
	return b[8:], n, nil
}

func (m *File) sampleTable(stbl map[string][][]byte, timescale uint32) ([]Sample, error) {
	stszB, err := one(stbl, "stsz")
	if err != nil {
		return nil, err
	}
	if len(stszB) < 12 {
		return nil, ErrIncomplete
	}
	fixed := binary.BigEndian.Uint32(stszB[4:])
	count := binary.BigEndian.Uint32(stszB[8:])
	if fixed == 0 && uint64(count)*4 > uint64(len(stszB)-12) {
		return nil, ErrIncomplete
	}
	if uint64(count) > uint64(m.size) {
		return nil, ErrIncomplete
	}
	samples := make([]Sample, count)
	for i := range samples {
		samples[i].Size = fixed
		if fixed == 0 {
			samples[i].Size = binary.BigEndian.Uint32(stszB[12+4*i:])
		}
	}

	// Times from stts.
	sttsB, err := one(stbl, "stts")
	if err != nil {
		return nil, err
	}
	stts, n, err := table(sttsB, 8)
	if err != nil {
		return nil, err
	}
	var ticks uint64
	i := 0
	for e := uint32(0); e < n && i < len(samples); e++ {
		cnt := binary.BigEndian.Uint32(stts[8*e:])
		delta := binary.BigEndian.Uint32(stts[8*e+4:])
		for c := uint32(0); c < cnt && i < len(samples); c++ {
			if timescale > 0 {
				samples[i].Start = float64(ticks) / float64(timescale)
				samples[i].Duration = float64(delta) / float64(timescale)
			}
			ticks += uint64(delta)
			i++
		}
	}

	// Chunk offsets from stco or co64.
	var offsets []int64
	if b, err := one(stbl, "co64"); err == nil {
		t, n, err := table(b, 8)
		if err != nil {
			return nil, err
		}
		for c := uint32(0); c < n; c++ {
			offsets = append(offsets, int64(binary.BigEndian.Uint64(t[8*c:])))
		}
	} else {
		b, err := one(stbl, "stco")
		if err != nil {
			return nil, err
		}
		t, n, err := table(b, 4)
		if err != nil {
			return nil, err
		}
		for c := uint32(0); c < n; c++ {
			offsets = append(offsets, int64(binary.BigEndian.Uint32(t[4*c:])))
		}
	}

	// Samples per chunk from stsc.
	stscB, err := one(stbl, "stsc")
	if err != nil {
		return nil, err
	}
	stsc, nEntries, err := table(stscB, 12)
	if err != nil {
		return nil, err
	}
	s := 0
	for e := uint32(0); e < nEntries && s < len(samples); e++ {
		first := binary.BigEndian.Uint32(stsc[12*e:])
		per := binary.BigEndian.Uint32(stsc[12*e+4:])
		last := uint32(len(offsets)) + 1
		if e+1 < nEntries {
			last = binary.BigEndian.Uint32(stsc[12*(e+1):])
		}
		if first == 0 || per == 0 {
			return nil, ErrIncomplete
		}
		for chunk := first; chunk < last && s < len(samples); chunk++ {
			if int(chunk-1) >= len(offsets) {
				return nil, ErrIncomplete
			}
			off := offsets[chunk-1]
			for k := uint32(0); k < per && s < len(samples); k++ {
				samples[s].Offset = off
				off += int64(samples[s].Size)
				s++
			}
		}
	}
	if s < len(samples) {
		return nil, ErrIncomplete
	}
	return samples, nil
}
