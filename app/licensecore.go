//go:build windows

package main

// licensecore.go: rebuilt from the v4.11.15 binary (the original source was lost).
// Every constant here was read out of the released exe, so machine codes and
// signed keys stay identical for existing customers. Do not change any value.
//
//   machine code : "ZB-" + base32(sha256("zanbooster|" + lower(trim(MachineGuid)))[:6]) split 5-5
//   signed keys  : "ZAN2.<b32>" -> 12-byte payload + 64-byte ed25519 signature
//                  "ZAN1.<b32>" or "ZAN-<b32>" -> 10-byte payload + signature
//   payload v2   : [0]=2 [1]=kind [2:6]=minutes since epoch (BE u32, 0 = never) [6:12]=machine bytes
//   payload v1   : [0]=1 [1]=kind [2:4]=days since epoch (BE u16, 0 = never)     [4:10]=machine bytes
//                  v1 expiry = end of that day in Thai time (epoch + days+1 days - 7h)

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"strings"
	"time"
)

const licPubKeyB64 = "3V1R/VXml9xfyClbNdmDXo1yVzrDtmjLNsD9igzmdnc="

var (
	licB32   = base32.StdEncoding.WithPadding(base32.NoPadding)
	licEpoch = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
)

func machineBytes(guid string) []byte {
	sum := sha256.Sum256([]byte("zanbooster|" + strings.ToLower(strings.TrimSpace(guid))))
	return sum[:6]
}

func machineCode(guid string) string {
	s := licB32.EncodeToString(machineBytes(guid))
	return "ZB-" + s[:5] + "-" + s[5:10]
}

var (
	errKeyShort   = errors.New("คีย์ไม่ถูกต้อง (คัดลอกมาไม่ครบ?)")
	errKeyBad     = errors.New("คีย์ไม่ถูกต้อง")
	errKeyMachine = errors.New("คีย์นี้เป็นของเครื่องอื่น (1 คีย์ใช้ได้ 1 เครื่อง)")
	errKeyNewer   = errors.New("คีย์นี้ต้องใช้กับโปรแกรมเวอร์ชันใหม่กว่า กรุณาอัปเดต")
)

// parseLicKey checks an offline signed key against this machine.
// Returns the plan kind byte and the expiry (zero time = never expires).
func parseLicKey(key string, mb []byte) (kind byte, exp time.Time, err error) {
	k := strings.ToUpper(strings.Join(strings.Fields(key), ""))
	var body string
	var n int
	switch {
	case strings.HasPrefix(k, "ZAN2."):
		body, n = k[5:], 12
	case strings.HasPrefix(k, "ZAN1."):
		body, n = k[5:], 10
	case strings.HasPrefix(k, "ZAN-"):
		body, n = k[4:], 10
	default:
		return 0, exp, errKeyBad
	}
	raw, e := licB32.DecodeString(strings.Replace(body, "-", "", -1))
	if e != nil || len(raw) != n+64 {
		return 0, exp, errKeyShort
	}
	pub, _ := base64.StdEncoding.DecodeString(licPubKeyB64)
	if !ed25519.Verify(ed25519.PublicKey(pub), raw[:n], raw[n:]) {
		return 0, exp, errKeyBad
	}
	if n == 12 {
		if raw[0] != 2 {
			return 0, exp, errKeyNewer
		}
		if string(raw[6:12]) != string(mb) {
			return 0, exp, errKeyMachine
		}
		if m := binary.BigEndian.Uint32(raw[2:6]); m != 0 {
			exp = licEpoch.Add(time.Duration(m) * time.Minute)
		}
		return raw[1], exp, nil
	}
	if raw[0] != 1 {
		return 0, exp, errKeyNewer
	}
	if string(raw[4:10]) != string(mb) {
		return 0, exp, errKeyMachine
	}
	if d := binary.BigEndian.Uint16(raw[2:4]); d != 0 {
		exp = licEpoch.AddDate(0, 0, int(d)+1).Add(-7 * time.Hour)
	}
	return raw[1], exp, nil
}

// isShortKey: คีย์สั้นที่ผู้ขายส่งให้ (ZAN-XXXXX-XXXXX-XXXXX-XXXXX) ต้องถามเซิร์ฟเวอร์เพื่อแลกเป็นคีย์ยาว
// (ถอดจาก exe 4.11.15: ตัดช่องว่าง ตัวใหญ่ ตัด "ZAN" ข้างหน้า ตัด "-" แล้วต้องเหลือ 20 ตัว)
func isShortKey(k string) bool {
	s := strings.ToUpper(strings.Join(strings.Fields(k), ""))
	s = strings.TrimPrefix(s, "ZAN")
	return len(strings.ReplaceAll(s, "-", "")) == 20
}
