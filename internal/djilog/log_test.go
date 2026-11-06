package djilog

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"encoding/json"
	"errors"
	"github.com/Julianlg752/skyhud/internal/djilog/djilogtest"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPrefixOffsets(t *testing.T) {
	for _, c := range []struct {
		version       byte
		field         uint64
		detail, start uint64
	}{
		{5, 5000, 5000, 12},
		{11, 5000, 5000, 100},
		{12, 5000, 100, 536},
		{14, 5000, 100, 5000},
	} {
		p := prefix{detailOffset: c.field, version: c.version}
		if p.details() != c.detail || p.records() != c.start {
			t.Errorf("v%d: details %d records %d, want %d %d", c.version, p.details(), p.records(), c.detail, c.start)
		}
	}
}

// encoder writes records the way DJI Fly does, for tests.
type encoder struct {
	version byte
	buf     bytes.Buffer
	keys    map[featurePoint]aesKey // for v13+ encrypted records
}

// record writes one record: type, length, payload (seed byte + XOR-ed
// content, AES-encrypted first for v13+ when a key is given), end byte.
func (e *encoder) record(typ byte, content []byte) {
	seed := byte(0x5a)
	body := content
	if k, ok := e.keys[featureFor(typ, e.version)]; ok && e.version >= 13 {
		pad := aes.BlockSize - len(content)%aes.BlockSize
		plain := append(append([]byte{}, content...), bytes.Repeat([]byte{byte(pad)}, pad)...)
		block, _ := aes.NewCipher(k.key)
		ct := make([]byte, len(plain))
		cipher.NewCBCEncrypter(block, k.iv).CryptBlocks(ct, plain)
		k.iv = ct[len(ct)-aes.BlockSize:]
		e.keys[featureFor(typ, e.version)] = k
		body = append(ct, 0) // the last payload byte is not part of the content
	}
	key := xorKey(seed, typ)
	payload := []byte{seed}
	for i, b := range body {
		payload = append(payload, b^key[i%8])
	}
	e.buf.WriteByte(typ)
	if e.version <= 12 {
		e.buf.WriteByte(byte(len(payload)))
	} else {
		binary.Write(&e.buf, binary.LittleEndian, uint16(len(payload)))
	}
	e.buf.Write(payload)
	e.buf.WriteByte(0xFF)
}

func osd(height, vx, vy float64) []byte {
	b := make([]byte, 16+2*7+4)
	binary.LittleEndian.PutUint16(b[16:], uint16(int16(height*10))) // the "altitude" field is height above home
	binary.LittleEndian.PutUint16(b[18:], uint16(int16(vx*10)))
	binary.LittleEndian.PutUint16(b[20:], uint16(int16(vy*10)))
	return b
}

func rc(aileron, elevator, throttle, rudder uint16) []byte {
	b := make([]byte, 13)
	for i, v := range []uint16{aileron, elevator, throttle, rudder, 1024} {
		binary.LittleEndian.PutUint16(b[2*i:], v)
	}
	return b
}

func custom(at time.Time) []byte {
	b := make([]byte, 18)
	binary.LittleEndian.PutUint64(b[10:], uint64(at.UnixMilli()))
	return b
}

func centerBattery(pct byte, volts float64) []byte {
	b := make([]byte, 40)
	b[0] = pct
	binary.LittleEndian.PutUint16(b[1:], uint16(volts*1000))
	return b
}

var t0 = time.Date(2025, 6, 1, 8, 30, 17, 0, time.UTC)

// v12Log builds a version 12 log: prefix, 436 bytes of details, records.
func v12Log(t *testing.T, aircraft string, frames int) []byte {
	t.Helper()
	var f bytes.Buffer
	pre := make([]byte, 100)
	binary.LittleEndian.PutUint64(pre, 0)
	pre[10] = 12
	f.Write(pre)
	details := make([]byte, 436)
	binary.LittleEndian.PutUint64(details[91:], uint64(t0.UnixMilli()))
	details[271] = 152 // Avata 2
	copy(details[280:312], aircraft)
	f.Write(details)
	e := &encoder{version: 12}
	for i := range frames {
		// A frame is an OSD record and the records after it.
		e.record(1, osd(float64(i)/10, 1.5, -2))
		e.record(5, custom(t0.Add(time.Duration(i)*100*time.Millisecond)))
		e.record(4, rc(1024, 1100, uint16(364+i), 1024))
		e.record(7, centerBattery(byte(90-i/10), 15.2))
	}
	f.Write(e.buf.Bytes())
	return f.Bytes()
}

func TestReadV12(t *testing.T) {
	l, err := Open(v12Log(t, "DJI Avata 2", 30))
	if err != nil {
		t.Fatal(err)
	}
	if l.Version != 12 || l.Aircraft != "DJI Avata 2" || !l.Start.Equal(t0) {
		t.Fatalf("log = v%d %q %v", l.Version, l.Aircraft, l.Start)
	}
	fs, err := l.Frames(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(fs) != 29 { // like dji-log-parser, the last OSD's frame is not emitted
		t.Fatalf("%d frames", len(fs))
	}
	f := fs[10]
	if math.Abs(f.Height-1.0) > 1e-6 || math.Abs(f.XSpeed-1.5) > 1e-6 || math.Abs(f.YSpeed+2) > 1e-6 {
		t.Fatalf("frame 10 = %+v", f)
	}
	if f.Throttle != 374 || f.Elevator != 1100 || f.Battery != 89 || math.Abs(f.Volts-15.2) > 1e-6 || !f.Time.Equal(t0.Add(time.Second)) {
		t.Fatalf("frame 10 = %+v", f)
	}
}

func TestAesRecord(t *testing.T) {
	k := aesKey{key: bytes.Repeat([]byte{7}, 32), iv: bytes.Repeat([]byte{9}, 16)}
	e := &encoder{version: 14, keys: map[featurePoint]aesKey{fpBase: k}}
	e.record(1, osd(12.3, 4, 5))
	e.record(1, osd(12.4, 4, 5)) // the IV chains from the previous record
	e.record(1, osd(12.5, 4, 5))
	keys := keychain{fpBase: k}
	recs, err := readRecords(e.buf.Bytes(), 14, [](keychain){keys})
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 3 {
		t.Fatalf("%d records", len(recs))
	}
	if h := float64(int16(binary.LittleEndian.Uint16(recs[2].data[16:]))) / 10; math.Abs(h-12.5) > 1e-9 {
		t.Fatalf("third record height %v", h)
	}
}

func TestTruncatedLog(t *testing.T) {
	b := v12Log(t, "DJI Avata 2", 10)
	for _, n := range []int{5, 120, len(b) - 7} {
		l, err := Open(b[:n])
		if err != nil {
			continue
		}
		if _, err := l.Frames(nil); err != nil && strings.Contains(err.Error(), "panic") {
			t.Fatal(err)
		}
	}
}

func TestHugeLength(t *testing.T) {
	// a v14 record claiming 65000 bytes in a 6-byte buffer
	recs, err := readRecords([]byte{1, 0xE8, 0xFD, 1, 2, 3}, 14, nil)
	if len(recs) != 0 || !errors.Is(err, ErrDamaged) {
		t.Fatalf("records %d, err %v", len(recs), err)
	}
}

func TestKeychainJSON(t *testing.T) {
	var got struct {
		Version    int `json:"version"`
		Department int `json:"department"`
		Keychains  [][]struct {
			FeaturePoint  string `json:"featurePoint"`
			AesCiphertext string `json:"aesCiphertext"`
		} `json:"keychainsArray"`
	}
	var header string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header = r.Header.Get("Api-Key")
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &got)
		w.Write([]byte(`{"result":{"code":0,"msg":"ok"},"data":[[{"featurePoint":"FR_Standardization_Feature_Base_1","aesKey":"BwcHBwcHBwcHBwcHBwcHBwcHBwcHBwcHBwcHBwcHBwc=","aesIv":"CQkJCQkJCQkJCQkJCQkJCQ=="}]]}`))
	}))
	defer srv.Close()
	req := KeychainRequest{Version: 1, Department: 3, Keychains: [][]EncodedKeyPoint{{{FeaturePoint: "FR_Standardization_Feature_Base_1", AesCiphertext: "AAEC"}}}}
	kc, err := FetchKeychains(t.Context(), srv.Client(), srv.URL, "KEY-XYZ", req)
	if err != nil {
		t.Fatal(err)
	}
	if header != "KEY-XYZ" || got.Department != 3 || got.Keychains[0][0].AesCiphertext != "AAEC" {
		t.Fatalf("request: header %q, body %+v", header, got)
	}
	if len(kc) != 1 || kc[0][0].FeaturePoint != "FR_Standardization_Feature_Base_1" || kc[0][0].AesIV != "CQkJCQkJCQkJCQkJCQkJCQ==" {
		t.Fatalf("keychains = %+v", kc)
	}
}

func TestFetchErrorsWithoutKeyLeak(t *testing.T) {
	for _, status := range []int{401, 403, 500} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			w.Write([]byte(`{"result":{"code":1,"msg":"bad key KEY-XYZ"}}`))
		}))
		_, err := FetchKeychains(t.Context(), srv.Client(), srv.URL, "KEY-XYZ", KeychainRequest{})
		srv.Close()
		if err == nil || strings.Contains(err.Error(), "KEY-XYZ") {
			t.Fatalf("status %d: err = %v", status, err)
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"result":{"code":3,"msg":"quota exceeded"}}`))
	}))
	defer srv.Close()
	if _, err := FetchKeychains(t.Context(), srv.Client(), srv.URL, "KEY-XYZ", KeychainRequest{}); err == nil || !strings.Contains(err.Error(), "quota") {
		t.Fatalf("API error not reported: %v", err)
	}
}

func TestNegativeRecordOffset(t *testing.T) {
	b := djilogtest.V14Header("DJI Avata 2", t0)
	binary.LittleEndian.PutUint64(b, 1<<63|5) // a damaged header: the offset turns negative as an int
	if _, err := Open(b); !errors.Is(err, ErrDamaged) {
		t.Fatalf("err = %v", err)
	}
}

func TestEndFromDetails(t *testing.T) {
	l, err := Open(djilogtest.V14Header("DJI Avata 2", t0))
	if err != nil {
		t.Fatal(err)
	}
	if !l.End.Equal(t0.Add(90 * time.Second)) {
		t.Fatalf("end = %v", l.End)
	}
}

func TestKeyNotSentOnRedirect(t *testing.T) {
	var leaked string
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaked = r.Header.Get("Api-Key")
	}))
	defer other.Close()
	dji := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL, http.StatusTemporaryRedirect)
	}))
	defer dji.Close()
	_, err := FetchKeychains(t.Context(), http.DefaultClient, dji.URL, "KEY-XYZ", KeychainRequest{})
	if leaked != "" || err == nil {
		t.Fatalf("key sent to the redirect target (%q), err %v", leaked, err)
	}
}

func TestEmptyKeychainsAreAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"result":{"code":0,"msg":"ok"},"data":[]}`))
	}))
	defer srv.Close()
	req := KeychainRequest{Keychains: [][]EncodedKeyPoint{{{FeaturePoint: "FR_Standardization_Feature_Base_1", AesCiphertext: "AA=="}}}}
	if _, err := FetchKeychains(t.Context(), srv.Client(), srv.URL, "K", req); err == nil {
		t.Fatal("an empty answer for a log with keys was accepted")
	}
}
