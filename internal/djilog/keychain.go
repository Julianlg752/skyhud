package djilog

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// DefaultEndpoint is DJI's keychain service (as in dji-log-parser).
const DefaultEndpoint = "https://dev.dji.com/openapi/v1/flight-records/keychains"

// featurePoint groups record types that share an AES key.
type featurePoint uint16

const (
	fpBase featurePoint = iota + 1
	fpVision
	fpWaypoint
	fpAgriculture
	fpAirLink
	fpAfterSales
	fpDJIFlyCustom
	fpPlaintext
	fpFlightHub
	fpGimbal
	fpRC
	fpCamera
	fpBattery
	fpFlySafe
	fpSecurity
)

var featureNames = []string{"", "Base_1", "Vision_2", "Waypoint_3", "Agriculture_4", "AirLink_5", "AfterSales_6",
	"DJIFlyCustom_7", "Plaintext_8", "FlightHub_9", "Gimbal_10", "RC_11", "Camera_12", "Battery_13", "FlySafe_14", "Security_15"}

func (f featurePoint) String() string {
	if int(f) < len(featureNames) && f > 0 {
		return "FR_Standardization_Feature_" + featureNames[f]
	}
	return fmt.Sprintf("FR_Standardization_Feature_Unknown_%d", f)
}

func parseFeature(s string) (featurePoint, bool) {
	for i, n := range featureNames {
		if i > 0 && s == "FR_Standardization_Feature_"+n {
			return featurePoint(i), true
		}
	}
	return 0, false
}

// featureFor maps a record type to its feature point, per log version.
func featureFor(typ, version byte) featurePoint {
	v13 := version == 13
	pick := func(at13, later featurePoint) featurePoint {
		if v13 {
			return at13
		}
		return later
	}
	switch typ {
	case 1, 2, 6, 13, 14, 15, 40, 58, 59, 63:
		return fpBase
	case 3:
		return pick(fpBase, fpGimbal)
	case 4, 11, 29, 33:
		return pick(fpBase, fpRC)
	case 5, 9, 10, 20, 24, 30, 54:
		return fpDJIFlyCustom
	case 7, 8:
		return pick(fpBase, fpBattery)
	case 12, 16, 19, 26, 27:
		return fpAfterSales
	case 17, 18:
		return fpVision
	case 21, 41, 43, 44, 45, 46, 47, 48:
		return fpAgriculture
	case 22:
		return pick(fpAfterSales, fpBattery)
	case 25:
		return pick(fpBase, fpCamera)
	case 28, 51, 52:
		return pick(fpAfterSales, fpFlySafe)
	case 31, 32, 34, 35, 36, 38, 39:
		return fpWaypoint
	case 49:
		return fpAirLink
	case 53:
		return pick(fpAfterSales, fpFlightHub)
	case 55:
		return fpSecurity
	case 62:
		return fpRC
	}
	return fpPlaintext
}

type aesKey struct{ key, iv []byte }

// keychain holds the current key and IV per feature point; the IV moves on
// with every record decrypted.
type keychain map[featurePoint]aesKey

func (k keychain) clone() keychain {
	out := keychain{}
	for f, v := range k {
		out[f] = v
	}
	return out
}

func newKeychain(points []KeyPoint) keychain {
	k := keychain{}
	for _, p := range points {
		f, ok := parseFeature(p.FeaturePoint)
		if !ok {
			continue
		}
		key, err1 := base64.StdEncoding.DecodeString(p.AesKey)
		iv, err2 := base64.StdEncoding.DecodeString(p.AesIV)
		if err1 == nil && err2 == nil {
			k[f] = aesKey{key: key, iv: iv}
		}
	}
	return k
}

// EncodedKeyPoint is one encrypted key stored in the log.
type EncodedKeyPoint struct {
	FeaturePoint  string `json:"featurePoint"`
	AesCiphertext string `json:"aesCiphertext"`
}

// KeychainRequest is the body sent to DJI's key service.
type KeychainRequest struct {
	Version    uint16              `json:"version"`
	Department uint8               `json:"department"`
	Keychains  [][]EncodedKeyPoint `json:"keychainsArray"`
}

// KeyPoint is one decrypted key from DJI's key service.
type KeyPoint struct {
	FeaturePoint string `json:"featurePoint"`
	AesKey       string `json:"aesKey"`
	AesIV        string `json:"aesIv"`
}

// FetchKeychains asks DJI's key service for a log's keys. The API key goes
// only into the request header; it never appears in returned errors.
func FetchKeychains(ctx context.Context, client *http.Client, endpoint, apiKey string, req KeychainRequest) ([][]KeyPoint, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("could not build the request to DJI's key service")
	}
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Api-Key", apiKey)
	// Never follow a redirect: Go would send the Api-Key header along.
	c := *client
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := c.Do(r)
	if err != nil {
		return nil, errors.New("could not reach DJI's key service")
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized:
		return nil, errors.New("DJI's key service rejected the DJI key")
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("DJI's key service answered HTTP %d", resp.StatusCode)
	}
	var out struct {
		Data   [][]KeyPoint `json:"data"`
		Result struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, errors.New("DJI's key service sent an answer SkyHUD can't read")
	}
	if out.Result.Code != 0 {
		msg := out.Result.Msg
		if apiKey != "" {
			msg = strings.ReplaceAll(msg, apiKey, "***")
		}
		return nil, fmt.Errorf("DJI's key service: %s", msg)
	}
	wanted := false
	for _, g := range req.Keychains {
		wanted = wanted || len(g) > 0
	}
	if out.Data == nil || wanted && len(out.Data) < len(req.Keychains) {
		return nil, errors.New("DJI's key service sent no keys for this log") // not cached, so it is asked again next time
	}
	return out.Data, nil
}

// KeychainCache keeps fetched keychains per log, so each log is sent to
// DJI's service once.
type KeychainCache struct{ Dir string }

// LogID names a log by its content.
func LogID(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func (c KeychainCache) path(id string) string { return filepath.Join(c.Dir, id+".json") }

// Get returns cached keychains for a log.
func (c KeychainCache) Get(id string) ([][]KeyPoint, bool) {
	b, err := os.ReadFile(c.path(id))
	if err != nil {
		return nil, false
	}
	var kc [][]KeyPoint
	if json.Unmarshal(b, &kc) != nil {
		return nil, false
	}
	return kc, true
}

// Put stores keychains for a log, readable by the user only.
func (c KeychainCache) Put(id string, kc [][]KeyPoint) error {
	if err := os.MkdirAll(c.Dir, 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(kc)
	if err != nil {
		return err
	}
	return os.WriteFile(c.path(id), b, 0o600)
}

// crc64 is the reflected CRC-64 used by dji-log-parser's crc64 crate (Jones
// polynomial, no final inversion), seeded with crc.
func crc64(crc uint64, data []byte) uint64 {
	for _, b := range data {
		crc = crcTable[byte(crc)^b] ^ (crc >> 8)
	}
	return crc
}

var crcTable = func() (t [256]uint64) {
	const poly = 0x95AC9329AC4BC9B5 // Jones, reflected
	for i := range t {
		c := uint64(i)
		for range 8 {
			if c&1 == 1 {
				c = c>>1 ^ poly
			} else {
				c >>= 1
			}
		}
		t[i] = c
	}
	return
}()
