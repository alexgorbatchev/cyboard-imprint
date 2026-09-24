// Package firmwaretest builds synthetic UF2 firmware files for tests.
package firmwaretest

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

const (
	magicStart0    = 0x0A324655
	magicStart1    = 0x9E5D5157
	magicEnd       = 0x0AB16F30
	rp2040FamilyID = 0xE48BFF56
	flashBase      = 0x10000000
	payloadSize    = 256
	blockSize      = 512
	headerSize     = 32
)

// ImprintDescriptor is the USB device descriptor QMK builds for VID 0x4359, PID 0x0000, bcdDevice 0x0023.
var ImprintDescriptor = []byte{0x12, 0x01, 0x00, 0x02, 0x00, 0x00, 0x00, 0x40, 0x59, 0x43, 0x00, 0x00, 0x23, 0x00, 0x01, 0x02, 0x03, 0x01}

// USBString encodes s as a USB string descriptor (bLength, 0x03, UTF-16LE).
func USBString(s string) []byte {
	out := []byte{byte(2 + 2*len(s)), 0x03}
	for _, c := range s {
		out = append(out, byte(c), 0)
	}
	return out
}

// WriteUF2 splits image into 256-byte RP2040 UF2 blocks at the flash base and writes them to a
// temporary file. The end magic of block corruptBlock is zeroed; pass -1 for a valid file.
func WriteUF2(t *testing.T, image []byte, corruptBlock int) string {
	t.Helper()
	numBlocks := (len(image) + payloadSize - 1) / payloadSize
	buf := new(bytes.Buffer)
	for i := range numBlocks {
		chunk := make([]byte, payloadSize)
		copy(chunk, image[i*payloadSize:min(len(image), (i+1)*payloadSize)])
		end := uint32(magicEnd)
		if i == corruptBlock {
			end = 0
		}
		header := []uint32{magicStart0, magicStart1, 0x00002000, flashBase + uint32(i*payloadSize), payloadSize, uint32(i), uint32(numBlocks), rp2040FamilyID}
		for _, v := range header {
			_ = binary.Write(buf, binary.LittleEndian, v)
		}
		buf.Write(chunk)
		buf.Write(make([]byte, blockSize-headerSize-4-payloadSize))
		_ = binary.Write(buf, binary.LittleEndian, end)
	}

	path := filepath.Join(t.TempDir(), "image.uf2")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("writing UF2: %v", err)
	}
	return path
}

// ImprintImage is a flash image carrying the Imprint (Patched) device descriptor and strings,
// placed so they straddle the first UF2 block boundary.
func ImprintImage() []byte {
	image := make([]byte, 250)
	image = append(image, ImprintDescriptor...)
	image = append(image, 0xAA, 0xBB)
	for _, s := range []string{"Cyboard", "Imprint (Patched)", "vial:f64c2b3c"} {
		image = append(image, USBString(s)...)
	}
	return append(image, make([]byte, 100)...)
}
