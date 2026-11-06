// Package djilog reads DJI Fly flight logs (DJIFlightRecord_*.txt), including
// the encrypted ones from version 13 on, for the parts the HUD needs: time,
// height and speed (to line the log up with the video), stick positions and
// battery.
//
// The format follows dji-log-parser by Luc Vauvillier and contributors (MIT
// license, https://github.com/lvauvillier/dji-log-parser, v0.5.7): the file
// prefix and details layout, the record framing, the XOR scrambling keyed by
// CRC-64, AES-256-CBC per feature point with keychains from DJI's API, and the
// OSD, RC, custom and battery record layouts. Serial numbers and positions in
// the log are never read out.
package djilog

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"strings"
	"time"
)

// ErrDamaged means the log ends early or a record doesn't fit.
var ErrDamaged = errors.New("flight log looks damaged or incomplete")

// ErrKeychainRequired means an encrypted log was read without keychains.
var ErrKeychainRequired = errors.New("flight log is encrypted; its keys come from DJI's key service")

// Log is an opened flight log.
type Log struct {
	Version    int
	Aircraft   string    // e.g. "DJI Avata 2"
	Product    int       // DJI product type number (152 = Avata 2)
	Start      time.Time // UTC, from the log's details
	End        time.Time // Start plus the flight time in the details
	data       []byte
	recStart   int
	auxVersion uint16
	department uint8
}

// Frame is the aircraft state at one OSD record.
type Frame struct {
	Time                                time.Time // UTC
	Height, XSpeed, YSpeed, ZSpeed      float64   // m above takeoff, m/s
	FlyTime                             float64   // s, the OSD's 0.1 s counter; -1 when the record is too short
	Aileron, Elevator, Throttle, Rudder int       // stick positions, about 364..1684
	Battery                             int       // percent
	Volts                               float64
	HasRC                               bool // an RC record was seen (motion controllers write none)
}

type prefix struct {
	detailOffset uint64
	version      byte
}

const (
	prefixSize    = 100
	oldPrefixSize = 12
	v12InfoSize   = 436
)

func (p prefix) details() uint64 {
	if p.version < 12 {
		return p.detailOffset
	}
	return prefixSize
}

func (p prefix) records() uint64 {
	switch {
	case p.version < 6:
		return oldPrefixSize
	case p.version < 12:
		return prefixSize
	case p.version == 12:
		return prefixSize + v12InfoSize
	}
	return p.detailOffset
}

// Open reads the prefix and details of a log.
func Open(b []byte) (*Log, error) {
	if len(b) < prefixSize {
		return nil, ErrDamaged
	}
	p := prefix{detailOffset: binary.LittleEndian.Uint64(b), version: b[10]}
	l := &Log{Version: int(p.version), data: b}
	d := p.details()
	if d >= uint64(len(b)) {
		return nil, ErrDamaged
	}
	if p.version < 13 {
		l.readDetails(b[d:])
		l.recStart = int(p.records())
	} else {
		pos := int(d)
		info, next, err := auxiliary(b, pos, 0)
		if err != nil {
			return nil, err
		}
		// info: version byte, u16 length, details, u16 length, signature
		if len(info) < 3 {
			return nil, ErrDamaged
		}
		n := int(binary.LittleEndian.Uint16(info[1:]))
		if 3+n > len(info) {
			return nil, ErrDamaged
		}
		l.readDetails(info[3 : 3+n])
		ver, after, err := auxiliary(b, next, 1)
		if err != nil {
			return nil, err
		}
		if len(ver) >= 3 {
			l.auxVersion = binary.LittleEndian.Uint16(ver)
			l.department = ver[2]
		}
		if r := p.records(); r == 0 {
			l.recStart = after
		} else if r > uint64(len(b)) {
			return nil, ErrDamaged
		} else {
			l.recStart = int(r)
		}
	}
	if l.recStart < 0 || l.recStart > len(b) {
		return nil, ErrDamaged
	}
	return l, nil
}

// auxiliary reads a v13+ auxiliary block (magic, u16 length, payload) at pos.
// Block 0 is XOR-scrambled like a record of type 0.
func auxiliary(b []byte, pos int, magic byte) ([]byte, int, error) {
	if pos+3 > len(b) || b[pos] != magic {
		return nil, 0, ErrDamaged
	}
	n := int(binary.LittleEndian.Uint16(b[pos+1:]))
	end := pos + 3 + n
	if end > len(b) {
		return nil, 0, ErrDamaged
	}
	payload := b[pos+3 : end]
	if magic == 0 {
		if len(payload) == 0 {
			return nil, 0, ErrDamaged
		}
		payload = unxor(payload, 0)
	}
	return payload, end, nil
}

// readDetails takes what the HUD needs from the details block (version 6
// and later layout): start time, product type and aircraft name.
func (l *Log) readDetails(d []byte) {
	if len(d) < 312 {
		return
	}
	ms := int64(binary.LittleEndian.Uint64(d[91:]))
	l.Start = time.UnixMilli(ms).UTC()
	l.End = l.Start.Add(time.Duration(int32(binary.LittleEndian.Uint32(d[119:]))) * time.Millisecond)
	l.Product = int(d[271])
	l.Aircraft = strings.TrimRight(string(d[280:312]), "\x00")
}

type rawRecord struct {
	typ  byte
	data []byte
}

// Record types used here.
const (
	recOSD           = 1
	recRC            = 4
	recCustom        = 5
	recCenterBattery = 7
	recSmartBattery  = 8
	recBatteryGroup  = 22
	recKeyRecover    = 50
	recKeyStorage    = 56
	recRCDisplay     = 62
)

// readRecords walks the records in b, decoding each with the keychain of the
// current group (a KeyStorageRecover record starts the next group).
func readRecords(b []byte, version int, groups []keychain) ([]rawRecord, error) {
	var out []rawRecord
	group := 0
	kc := keychain{}
	if len(groups) > 0 {
		kc = groups[0].clone()
	}
	lenSize := 2
	if version <= 12 {
		lenSize = 1
	}
	pos := 0
	for pos < len(b) {
		if pos+1 < len(b) && b[pos] == 0xFF && b[pos+1] == 0xD8 { // embedded JPEG
			end := indexPair(b, pos+2, 0xFF, 0xD9)
			if end < 0 {
				break
			}
			pos = end + 2
			continue
		}
		if pos+1+lenSize > len(b) {
			return out, ErrDamaged
		}
		typ := b[pos]
		n := int(b[pos+1])
		if lenSize == 2 {
			n = int(binary.LittleEndian.Uint16(b[pos+1:]))
		}
		start := pos + 1 + lenSize
		end := start + n
		if end >= len(b) {
			return out, ErrDamaged
		}
		if b[end] != 0xFF { // not a record here: skip to the next end byte
			next := indexByte(b, pos+1, 0xFF)
			if next < 0 {
				break
			}
			pos = next + 1
			continue
		}
		payload := b[start:end]
		pos = end + 1
		if typ == recKeyRecover {
			group++
			kc = keychain{}
			if group < len(groups) {
				kc = groups[group].clone()
			}
			continue
		}
		out = append(out, rawRecord{typ, decode(payload, typ, version, kc)})
	}
	return out, nil
}

func decode(payload []byte, typ byte, version int, kc keychain) []byte {
	if version <= 6 || len(payload) == 0 {
		return payload
	}
	x := unxor(payload, typ)
	if version < 13 {
		return x
	}
	fp := featureFor(typ, byte(version))
	k, ok := kc[fp]
	if fp == fpPlaintext || !ok {
		return x
	}
	n := len(payload) - 2 // the seed byte and the last byte are not content
	if n <= 0 || n > len(x) || n%aes.BlockSize != 0 || len(k.key) != 32 || len(k.iv) != aes.BlockSize {
		return nil
	}
	ct := append([]byte{}, x[:n]...)
	block, err := aes.NewCipher(k.key)
	if err != nil {
		return nil
	}
	kc[fp] = aesKey{key: k.key, iv: append([]byte{}, ct[n-aes.BlockSize:]...)}
	plain := make([]byte, n)
	cipher.NewCBCDecrypter(block, k.iv).CryptBlocks(plain, ct)
	pad := int(plain[n-1])
	if pad == 0 || pad > aes.BlockSize {
		return nil
	}
	return plain[:n-pad]
}

// unxor undoes DJI's scrambling: the first byte seeds an 8-byte key.
func unxor(payload []byte, typ byte) []byte {
	key := xorKey(payload[0], typ)
	out := make([]byte, len(payload)-1)
	for i, b := range payload[1:] {
		out[i] = b ^ key[i%8]
	}
	return out
}

const xorMagic uint64 = 0x123456789ABCDEF0

func xorKey(seed, typ byte) [8]byte {
	var data [8]byte
	binary.LittleEndian.PutUint64(data[:], xorMagic*uint64(seed))
	var key [8]byte
	binary.LittleEndian.PutUint64(key[:], crc64(uint64(seed+typ), data[:]))
	return key
}

func indexByte(b []byte, from int, c byte) int {
	for i := from; i < len(b); i++ {
		if b[i] == c {
			return i
		}
	}
	return -1
}

func indexPair(b []byte, from int, c0, c1 byte) int {
	for i := from; i+1 < len(b); i++ {
		if b[i] == c0 && b[i+1] == c1 {
			return i
		}
	}
	return -1
}

// KeychainRequest lists the encrypted keys stored in the log, to send to
// DJI's key service. Logs before version 13 need none.
func (l *Log) KeychainRequest() (KeychainRequest, error) {
	req := KeychainRequest{Version: l.auxVersion, Department: l.department}
	if req.Department == 0 || req.Department > 8 {
		req.Department = 3 // DJI Fly
	}
	if l.Version < 13 {
		return req, nil
	}
	var group []EncodedKeyPoint
	recs, err := readRecordsKeepingRecover(l.data[l.recStart:], l.Version)
	for _, r := range recs {
		switch r.typ {
		case recKeyStorage:
			if len(r.data) < 4 {
				continue
			}
			fp := featurePoint(binary.LittleEndian.Uint16(r.data))
			n := int(binary.LittleEndian.Uint16(r.data[2:]))
			if 4+n > len(r.data) {
				continue
			}
			group = append(group, EncodedKeyPoint{FeaturePoint: fp.String(), AesCiphertext: base64.StdEncoding.EncodeToString(r.data[4 : 4+n])})
		case recKeyRecover:
			req.Keychains = append(req.Keychains, group)
			group = nil
		}
	}
	req.Keychains = append(req.Keychains, group)
	if err != nil && len(recs) == 0 {
		return req, err
	}
	return req, nil
}

// readRecordsKeepingRecover is readRecords without keys, keeping the group
// boundaries as records.
func readRecordsKeepingRecover(b []byte, version int) ([]rawRecord, error) {
	var out []rawRecord
	lenSize := 2
	if version <= 12 {
		lenSize = 1
	}
	pos := 0
	for pos < len(b) {
		if pos+1 < len(b) && b[pos] == 0xFF && b[pos+1] == 0xD8 {
			end := indexPair(b, pos+2, 0xFF, 0xD9)
			if end < 0 {
				break
			}
			pos = end + 2
			continue
		}
		if pos+1+lenSize > len(b) {
			return out, ErrDamaged
		}
		typ := b[pos]
		n := int(b[pos+1])
		if lenSize == 2 {
			n = int(binary.LittleEndian.Uint16(b[pos+1:]))
		}
		end := pos + 1 + lenSize + n
		if end >= len(b) {
			return out, ErrDamaged
		}
		if b[end] != 0xFF {
			next := indexByte(b, pos+1, 0xFF)
			if next < 0 {
				break
			}
			pos = next + 1
			continue
		}
		payload := b[pos+1+lenSize : end]
		pos = end + 1
		switch typ {
		case recKeyRecover:
			out = append(out, rawRecord{typ, nil})
		case recKeyStorage:
			if len(payload) > 0 {
				out = append(out, rawRecord{typ, unxor(payload, typ)})
			}
		}
	}
	return out, nil
}

// Frames decodes the log into one frame per OSD record. Encrypted logs
// (version 13 on) need the keychains from DJI's key service. Like
// dji-log-parser, a frame is emitted when the next OSD record starts, so the
// last OSD record has none. Reading stops at a damaged or cut-off record;
// ErrDamaged comes back only when no frame could be read.
func (l *Log) Frames(keychains [][]KeyPoint) ([]Frame, error) {
	if l.Version >= 13 && keychains == nil {
		return nil, ErrKeychainRequired
	}
	var groups []keychain
	for _, kp := range keychains {
		groups = append(groups, newKeychain(kp))
	}
	recs, err := readRecords(l.data[l.recStart:], l.Version, groups)
	var frames []Frame
	var f Frame
	osds := 0
	for _, r := range recs {
		d := r.data
		switch r.typ {
		case recOSD:
			if len(d) < 24 {
				continue
			}
			if osds > 0 {
				frames = append(frames, f)
			}
			f.Height = float64(int16(binary.LittleEndian.Uint16(d[16:]))) / 10
			f.XSpeed = float64(int16(binary.LittleEndian.Uint16(d[18:]))) / 10
			f.YSpeed = float64(int16(binary.LittleEndian.Uint16(d[20:]))) / 10
			f.ZSpeed = float64(int16(binary.LittleEndian.Uint16(d[22:]))) / 10
			f.FlyTime = -1
			if len(d) >= 44 {
				f.FlyTime = float64(binary.LittleEndian.Uint16(d[42:])) / 10
			}
			osds++
		case recRC, recRCDisplay:
			off := 0
			if r.typ == recRCDisplay {
				off = 7
			}
			if len(d) < off+8 {
				continue
			}
			f.Aileron = int(binary.LittleEndian.Uint16(d[off:]))
			f.Elevator = int(binary.LittleEndian.Uint16(d[off+2:]))
			f.Throttle = int(binary.LittleEndian.Uint16(d[off+4:]))
			f.Rudder = int(binary.LittleEndian.Uint16(d[off+6:]))
			f.HasRC = true
		case recCustom:
			if len(d) < 18 {
				continue
			}
			ms := int64(binary.LittleEndian.Uint64(d[10:]))
			if t := time.UnixMilli(ms).UTC(); t.Year() > 2010 && t.Year() < 2100 {
				f.Time = t
			}
		case recCenterBattery:
			if len(d) < 3 {
				continue
			}
			f.Battery = int(d[0])
			f.Volts = float64(binary.LittleEndian.Uint16(d[1:])) / 1000
		case recSmartBattery:
			if len(d) < 27 {
				continue
			}
			f.Volts = float64(binary.LittleEndian.Uint16(d[24:])) / 1000
			f.Battery = int(d[26])
		case recBatteryGroup:
			if len(d) < 22 || d[0] != 2 { // 2: dynamic state
				continue
			}
			f.Volts = float64(int32(binary.LittleEndian.Uint32(d[2:]))) / 1000
			f.Battery = int(d[21])
		}
	}
	if len(frames) > 0 {
		err = nil // real logs end with a cut-off record; dji-log-parser stops there too
	}
	return frames, err
}
