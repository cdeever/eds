package image

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fake(project, version string, elf []byte) []byte {
	b := make([]byte, 4096)
	b[0] = imageMagic
	binary.LittleEndian.PutUint32(b[descAt:], descMagic)
	copy(b[versionAt:], version)
	copy(b[projectAt:], project)
	copy(b[elfAt:], elf)
	return b
}

func TestDescribeReadsTheDescription(t *testing.T) {
	d, err := Describe(fake("lp-stand", "f5fd6f9-dirty", []byte{0xde, 0xad, 0xbe, 0xef, 0, 1, 2, 3, 9, 9}))
	if err != nil {
		t.Fatal(err)
	}
	want := Desc{Project: "lp-stand", Version: "f5fd6f9-dirty", ELF: "deadbeef00010203"}
	if d != want {
		t.Errorf("Describe = %+v, want %+v", d, want)
	}
}

func TestDescribeRefusesWhatIsNotAnImage(t *testing.T) {
	b := fake("lp-stand", "v", nil)
	for name, bad := range map[string][]byte{
		"too short":    b[:100],
		"not an image": append([]byte{0x7f}, b[1:]...),
		"no description": func() []byte {
			c := append([]byte(nil), b...)
			binary.LittleEndian.PutUint32(c[descAt:], 0)
			return c
		}(),
	} {
		if _, err := Describe(bad); err == nil {
			t.Errorf("%s: described", name)
		}
	}
}

// The offsets are ESP-IDF's, so they are checked against a build when there
// is one: make build in a firmware directory first.
func TestDescribeReadsARealBuild(t *testing.T) {
	for _, project := range []string{"lp-stand", "cover-screen", "power-beacon"} {
		path := filepath.Join("..", "..", "..", "..", "firmware", project, "build", project+".bin")
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		d, err := Describe(b)
		if err != nil {
			t.Errorf("%s: %v", project, err)
			continue
		}
		if d.Project != project || d.Version == "" || len(d.ELF) != 16 || strings.Trim(d.ELF, "0") == "" {
			t.Errorf("%s: described as %+v", project, d)
		}
	}
}
