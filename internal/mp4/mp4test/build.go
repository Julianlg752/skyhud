// Package mp4test builds small synthetic MP4 files for tests.
package mp4test

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Options describes the single-track MP4 that Build writes.
type Options struct {
	Name            string // file name inside a temp dir; defaults to "clip.mp4"
	Dir             string // directory to write into; defaults to t.TempDir()
	CreationTime    time.Time
	Format          string // sample entry fourcc, e.g. "gpmd" or "hvc1"
	Timescale       uint32
	SampleDurations []uint32 // one per payload, in Timescale units
	Payloads        [][]byte
	SamplesPerChunk int // defaults to 1
	Use64BitOffsets bool
	TruncateBytes   int // bytes cut from the end of the file
}

var mp4Epoch = time.Date(1904, 1, 1, 0, 0, 0, 0, time.UTC)

// Build writes the file and returns its path.
func Build(t testing.TB, opt Options) string {
	t.Helper()
	if opt.SamplesPerChunk == 0 {
		opt.SamplesPerChunk = 1
	}
	if opt.Name == "" {
		opt.Name = "clip.mp4"
	}
	if opt.Dir == "" {
		opt.Dir = t.TempDir()
	}
	ftyp := box("ftyp", []byte("isom\x00\x00\x02\x00isomiso2mp41"))
	// The moov size doesn't depend on offset values, so build it once to measure.
	moovLen := len(moov(opt, make([]uint64, chunkCount(opt))))
	dataStart := uint64(len(ftyp) + moovLen + 8)
	offsets := make([]uint64, 0, chunkCount(opt))
	pos := dataStart
	for i, p := range opt.Payloads {
		if i%opt.SamplesPerChunk == 0 {
			offsets = append(offsets, pos)
		}
		pos += uint64(len(p))
	}
	var mdat bytes.Buffer
	for _, p := range opt.Payloads {
		mdat.Write(p)
	}
	var out bytes.Buffer
	out.Write(ftyp)
	out.Write(moov(opt, offsets))
	out.Write(box("mdat", mdat.Bytes()))
	b := out.Bytes()
	if opt.TruncateBytes > 0 {
		b = b[:len(b)-opt.TruncateBytes]
	}
	path := filepath.Join(opt.Dir, opt.Name)
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func chunkCount(opt Options) int {
	return (len(opt.Payloads) + opt.SamplesPerChunk - 1) / opt.SamplesPerChunk
}

func moov(opt Options, offsets []uint64) []byte {
	var total uint32
	for _, d := range opt.SampleDurations {
		total += d
	}
	created := uint32(opt.CreationTime.Sub(mp4Epoch) / time.Second)
	handler, name := "vide", "VideoHandler"
	if opt.Format == "gpmd" || opt.Format == "djmd" {
		handler, name = "meta", "GoPro MET"
	}

	mvhd := cat(u32(0), u32(created), u32(created), u32(opt.Timescale), u32(total), u32(0x00010000), u16(0x0100), make([]byte, 10), matrix(), make([]byte, 24), u32(2))
	tkhd := cat(u32(7), u32(created), u32(created), u32(1), u32(0), u32(total), make([]byte, 8), u16(0), u16(0), u16(0), u16(0), matrix(), u32(0), u32(0))
	mdhd := cat(u32(0), u32(created), u32(created), u32(opt.Timescale), u32(total), u16(0x55c4), u16(0))
	hdlr := cat(u32(0), u32(0), []byte(handler), make([]byte, 12), []byte(name), []byte{0})
	entry := cat(u32(16), []byte(opt.Format), make([]byte, 6), u16(1))
	stsd := cat(u32(0), u32(1), entry)

	stts := cat(u32(0), u32(uint32(len(opt.SampleDurations))))
	for _, d := range opt.SampleDurations {
		stts = cat(stts, u32(1), u32(d))
	}
	stsc := cat(u32(0), u32(1), u32(1), u32(uint32(opt.SamplesPerChunk)), u32(1))
	stsz := cat(u32(0), u32(0), u32(uint32(len(opt.Payloads))))
	for _, p := range opt.Payloads {
		stsz = cat(stsz, u32(uint32(len(p))))
	}
	var chunkBox []byte
	if opt.Use64BitOffsets {
		c := cat(u32(0), u32(uint32(len(offsets))))
		for _, o := range offsets {
			c = cat(c, u64(o))
		}
		chunkBox = box("co64", c)
	} else {
		c := cat(u32(0), u32(uint32(len(offsets))))
		for _, o := range offsets {
			c = cat(c, u32(uint32(o)))
		}
		chunkBox = box("stco", c)
	}
	stbl := box("stbl", cat(box("stsd", stsd), box("stts", stts), box("stsc", stsc), box("stsz", stsz), chunkBox))
	minf := box("minf", stbl)
	mdia := box("mdia", cat(box("mdhd", mdhd), box("hdlr", hdlr), minf))
	trak := box("trak", cat(box("tkhd", tkhd), mdia))
	return box("moov", cat(box("mvhd", mvhd), trak))
}

func matrix() []byte {
	return cat(u32(0x00010000), u32(0), u32(0), u32(0), u32(0x00010000), u32(0), u32(0), u32(0), u32(0x40000000))
}

func box(typ string, payload []byte) []byte {
	return cat(u32(uint32(8+len(payload))), []byte(typ), payload)
}

func cat(parts ...[]byte) []byte {
	var b []byte
	for _, p := range parts {
		b = append(b, p...)
	}
	return b
}

func u16(v uint16) []byte { return binary.BigEndian.AppendUint16(nil, v) }
func u32(v uint32) []byte { return binary.BigEndian.AppendUint32(nil, v) }
func u64(v uint64) []byte { return binary.BigEndian.AppendUint64(nil, v) }
