// Package image reads what an ESP-IDF application image says about itself.
//
// Every app image ESP-IDF builds carries an esp_app_desc_t right after its
// image header and first segment header: the project's name, its version
// and the SHA-256 of the ELF it was made from. The sender reads it so that a
// build is never offered to a device it is not for, and so that what the
// device says it runs afterward can be compared with what was sent.
package image

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
)

// The image header is 24 bytes and a segment header 8; the description is
// the first thing in the first segment.
const (
	descAt    = 24 + 8
	descMagic = 0xABCD5432

	versionAt = descAt + 16 // after magic, secure_version and two reserved words
	projectAt = versionAt + 32
	elfAt     = projectAt + 32 + 16 + 16 + 32 // after project_name, time, date, idf_ver
	descEnd   = elfAt + 32

	imageMagic = 0xE9
)

// Desc is what an image says about itself.
type Desc struct {
	Project string // esp_app_desc_t.project_name, e.g. "lp-stand"
	Version string // esp_app_desc_t.version, from git describe
	ELF     string // the first eight bytes of the ELF's SHA-256, in hex, as the device reports it
}

// Describe reads an app image's description.
func Describe(b []byte) (Desc, error) {
	if len(b) < descEnd {
		return Desc{}, errors.New("image: too short to be an ESP-IDF app image")
	}
	if b[0] != imageMagic {
		return Desc{}, fmt.Errorf("image: starts with 0x%02x, not an ESP-IDF app image", b[0])
	}
	if m := binary.LittleEndian.Uint32(b[descAt:]); m != descMagic {
		return Desc{}, fmt.Errorf("image: no app description (0x%08x where 0x%08x belongs)", m, descMagic)
	}
	return Desc{
		Project: cString(b[projectAt : projectAt+32]),
		Version: cString(b[versionAt : versionAt+32]),
		ELF:     hex.EncodeToString(b[elfAt : elfAt+8]),
	}, nil
}

func cString(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return string(b)
}
