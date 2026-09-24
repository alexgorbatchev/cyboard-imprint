package firmware

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/alexgorbatchev/mouse-issues/internal/firmware/firmwaretest"
)

func createTestUF2File(t *testing.T, familyID uint32) string {
	t.Helper()
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.uf2")

	buf := new(bytes.Buffer)
	// UF2 Block format:
	// 0: magicStart0 (0x0A324655)
	// 4: magicStart1 (0x9E5D5157)
	// 8: flags (0x00002000)
	// 12: targetAddr (0x10000000)
	// 16: payloadSize (256)
	// 20: blockNo (0)
	// 24: numBlocks (1)
	// 28: familyID (0xe48bff56)
	_ = binary.Write(buf, binary.LittleEndian, uint32(0x0A324655))
	_ = binary.Write(buf, binary.LittleEndian, uint32(0x9E5D5157))
	_ = binary.Write(buf, binary.LittleEndian, uint32(0x00002000))
	_ = binary.Write(buf, binary.LittleEndian, uint32(0x10000000))
	_ = binary.Write(buf, binary.LittleEndian, uint32(256))
	_ = binary.Write(buf, binary.LittleEndian, uint32(0))
	_ = binary.Write(buf, binary.LittleEndian, uint32(1))
	_ = binary.Write(buf, binary.LittleEndian, familyID)

	// Pad to 512 bytes (standard UF2 block size)
	pad := make([]byte, 512-32-4)
	buf.Write(pad)
	// magicEnd (0x0AB16F30)
	_ = binary.Write(buf, binary.LittleEndian, uint32(0x0AB16F30))

	if err := os.WriteFile(filePath, buf.Bytes(), 0644); err != nil {
		t.Fatalf("writing test UF2: %v", err)
	}
	return filePath
}

func TestInspectUF2_ValidRP2040(t *testing.T) {
	filePath := createTestUF2File(t, 0xe48bff56)

	info, err := InspectUF2(filePath)
	if err != nil {
		t.Fatalf("InspectUF2 failed: %v", err)
	}

	if !info.IsValidMagic {
		t.Errorf("expected valid magic, got false")
	}
	if !info.IsRP2040 {
		t.Errorf("expected RP2040 chip, got false")
	}
	if info.TargetAddr != 0x10000000 {
		t.Errorf("expected targetAddr 0x10000000, got 0x%x", info.TargetAddr)
	}
	if info.PayloadSize != 256 {
		t.Errorf("expected payloadSize 256, got %d", info.PayloadSize)
	}
}

func TestInspectUF2_InvalidMagic(t *testing.T) {
	tmpDir := t.TempDir()
	badFile := filepath.Join(tmpDir, "bad.uf2")
	_ = os.WriteFile(badFile, []byte("hello world not a uf2 file"), 0644)

	info, err := InspectUF2(badFile)
	if err == nil && info.IsValidMagic {
		t.Fatalf("expected error or invalid magic for bad file")
	}
}

func imageWith(parts ...[]byte) []byte {
	image := make([]byte, 300) // puts the descriptors across the first block boundary
	for _, p := range parts {
		image = append(image, p...)
	}
	return append(image, make([]byte, 100)...)
}

func TestInspectUF2_USBIdentity(t *testing.T) {
	path := firmwaretest.WriteUF2(t, firmwaretest.ImprintImage(), -1)

	info, err := InspectUF2(path)
	if err != nil {
		t.Fatalf("InspectUF2 failed: %v", err)
	}
	if info.BlockCount != 2 {
		t.Fatalf("expected 2 blocks (descriptor straddles the first boundary), got %d", info.BlockCount)
	}
	usb := info.USB
	if usb == nil {
		t.Fatal("expected a USB identity")
	}
	if usb.VendorID != 0x4359 || usb.ProductID != 0 || usb.Version != 0x0023 {
		t.Fatalf("unexpected USB identity: %+v", usb)
	}
	if want := []string{"Cyboard", "Imprint (Patched)", "vial:f64c2b3c"}; !slices.Equal(usb.Strings, want) {
		t.Fatalf("strings = %q, want %q", usb.Strings, want)
	}
}

func TestInspectUF2_WithoutUSBIdentity(t *testing.T) {
	info, err := InspectUF2(firmwaretest.WriteUF2(t, imageWith(), -1))
	if err != nil {
		t.Fatalf("InspectUF2 failed: %v", err)
	}
	if info.USB != nil {
		t.Fatalf("expected no USB identity, got %+v", info.USB)
	}
}

func TestInspectUF2_Rejects(t *testing.T) {
	other := slices.Clone(firmwaretest.ImprintDescriptor)
	other[12] = 0x22 // a second descriptor with a different bcdDevice

	tests := []struct {
		name    string
		path    string
		wantErr string
	}{
		{"ambiguous device descriptors", firmwaretest.WriteUF2(t, imageWith(firmwaretest.ImprintDescriptor, other), -1), "device descriptors"},
		{"corrupt block", firmwaretest.WriteUF2(t, imageWith(firmwaretest.ImprintDescriptor), 1), "block 1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := InspectUF2(tt.path)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}
