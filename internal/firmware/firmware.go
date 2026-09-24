package firmware

import (
	"encoding/binary"
	"fmt"
	"os"
	"slices"
	"strings"
)

const (
	UF2MagicStart0 = 0x0A324655
	UF2MagicStart1 = 0x9E5D5157
	UF2MagicEnd    = 0x0AB16F30
	RP2040FamilyID = 0xE48BFF56
)

const (
	uf2BlockSize        = 512
	uf2MaxPayload       = 476
	uf2FlagNotMainFlash = 0x00000001
	maxImageSpan        = 16 << 20 // RP2040 addresses at most 16 MiB of external flash
)

// USBIdentity is the USB device identity compiled into a firmware image.
type USBIdentity struct {
	VendorID  uint32   `json:"vendor_id"`
	ProductID uint32   `json:"product_id"`
	Version   uint32   `json:"version"` // bcdDevice
	Strings   []string `json:"strings"` // USB string descriptors in image order (manufacturer, product, serial)
}

// Info contains parsed metadata from a UF2 firmware binary.
type Info struct {
	FilePath     string       `json:"file_path"`
	FileSize     int64        `json:"file_size"`
	IsValidMagic bool         `json:"is_valid_magic"`
	IsRP2040     bool         `json:"is_rp2040"`
	Architecture string       `json:"architecture"`
	TargetAddr   uint32       `json:"target_addr"`
	PayloadSize  uint32       `json:"payload_size"`
	BlockCount   uint32       `json:"block_count"`
	FamilyID     uint32       `json:"family_id"`
	USB          *USBIdentity `json:"usb,omitempty"` // nil when the image has no USB device descriptor
}

type uf2Block struct {
	flags       uint32
	targetAddr  uint32
	payloadSize uint32
	numBlocks   uint32
	familyID    uint32
	payload     []byte
}

func parseUF2Block(b []byte) (uf2Block, bool) {
	le := binary.LittleEndian
	if le.Uint32(b[0:4]) != UF2MagicStart0 || le.Uint32(b[4:8]) != UF2MagicStart1 || le.Uint32(b[508:512]) != UF2MagicEnd {
		return uf2Block{}, false
	}
	blk := uf2Block{
		flags:       le.Uint32(b[8:12]),
		targetAddr:  le.Uint32(b[12:16]),
		payloadSize: le.Uint32(b[16:20]),
		numBlocks:   le.Uint32(b[24:28]),
		familyID:    le.Uint32(b[28:32]),
	}
	if blk.payloadSize > uf2MaxPayload {
		return uf2Block{}, false
	}
	blk.payload = b[32 : 32+blk.payloadSize]
	return blk, true
}

// InspectUF2 validates every block of a UF2 firmware file and extracts the USB identity
// compiled into the flash image.
func InspectUF2(filePath string) (*Info, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("opening UF2 file: %w", err)
	}

	info := &Info{FilePath: filePath, FileSize: int64(len(data))}
	if len(data) < uf2BlockSize {
		return info, fmt.Errorf("file too small for UF2 block (%d bytes, minimum %d)", len(data), uf2BlockSize)
	}

	le := binary.LittleEndian
	first, ok := parseUF2Block(data[:uf2BlockSize])
	if !ok {
		return info, fmt.Errorf("invalid UF2 magic header: 0x%08x 0x%08x", le.Uint32(data[0:4]), le.Uint32(data[4:8]))
	}
	info.IsValidMagic = true
	info.FamilyID = first.familyID
	info.IsRP2040 = first.familyID == RP2040FamilyID
	info.Architecture = "Unknown"
	if info.IsRP2040 {
		info.Architecture = "Raspberry Pi RP2040"
	}
	info.TargetAddr = first.targetAddr
	info.PayloadSize = first.payloadSize
	info.BlockCount = first.numBlocks

	if len(data)%uf2BlockSize != 0 {
		return info, fmt.Errorf("file size %d is not a multiple of the %d-byte UF2 block size", len(data), uf2BlockSize)
	}
	if got := uint32(len(data) / uf2BlockSize); got != first.numBlocks {
		return info, fmt.Errorf("file holds %d blocks but the header declares %d", got, first.numBlocks)
	}

	image, err := assembleImage(data)
	if err != nil {
		return info, err
	}
	usb, err := findUSBIdentity(image)
	if err != nil {
		return info, err
	}
	info.USB = usb
	return info, nil
}

// assembleImage validates every block and lays their main-flash payloads out by address.
func assembleImage(data []byte) ([]byte, error) {
	var blocks []uf2Block
	base, end := uint32(0xFFFFFFFF), uint32(0)
	for i := 0; i < len(data)/uf2BlockSize; i++ {
		blk, ok := parseUF2Block(data[i*uf2BlockSize : (i+1)*uf2BlockSize])
		if !ok {
			return nil, fmt.Errorf("block %d: invalid UF2 magic or payload size", i)
		}
		if blk.flags&uf2FlagNotMainFlash != 0 {
			continue
		}
		blocks = append(blocks, blk)
		base = min(base, blk.targetAddr)
		end = max(end, blk.targetAddr+blk.payloadSize)
	}
	if len(blocks) == 0 {
		return nil, nil
	}
	if end-base > maxImageSpan {
		return nil, fmt.Errorf("blocks span %d bytes, more than %d bytes of flash", end-base, maxImageSpan)
	}

	image := make([]byte, end-base)
	for _, blk := range blocks {
		copy(image[blk.targetAddr-base:], blk.payload)
	}
	return image, nil
}

// findUSBIdentity locates the 18-byte USB device descriptor and the USB string descriptors
// in a flash image. It returns nil when there is no device descriptor.
func findUSBIdentity(image []byte) (*USBIdentity, error) {
	le := binary.LittleEndian
	var found []USBIdentity
	for i := 0; i+18 <= len(image); i++ {
		d := image[i : i+18]
		if d[0] != 0x12 || d[1] != 0x01 {
			continue
		}
		bcdUSB := le.Uint16(d[2:4])
		maxPacket := d[7]
		validUSB := bcdUSB == 0x0110 || bcdUSB == 0x0200 || bcdUSB == 0x0201 || bcdUSB == 0x0210
		validPacket := maxPacket == 8 || maxPacket == 16 || maxPacket == 32 || maxPacket == 64
		if !validUSB || !validPacket || d[17] == 0 {
			continue
		}
		id := USBIdentity{VendorID: uint32(le.Uint16(d[8:10])), ProductID: uint32(le.Uint16(d[10:12])), Version: uint32(le.Uint16(d[12:14]))}
		if !slices.ContainsFunc(found, func(f USBIdentity) bool {
			return f.VendorID == id.VendorID && f.ProductID == id.ProductID && f.Version == id.Version
		}) {
			found = append(found, id)
		}
	}

	switch len(found) {
	case 0:
		return nil, nil
	case 1:
		found[0].Strings = findUSBStrings(image)
		return &found[0], nil
	default:
		return nil, fmt.Errorf("image contains %d different USB device descriptors", len(found))
	}
}

// findUSBStrings returns printable ASCII USB string descriptors (bLength, 0x03, UTF-16LE).
func findUSBStrings(image []byte) []string {
	var out []string
	for i := 0; i+4 <= len(image); i++ {
		n := int(image[i])
		if image[i+1] != 0x03 || n < 8 || n%2 != 0 || i+n > len(image) {
			continue
		}
		var sb strings.Builder
		ok := true
		for j := i + 2; j < i+n; j += 2 {
			c := image[j]
			if image[j+1] != 0 || c < 0x20 || c > 0x7e {
				ok = false
				break
			}
			sb.WriteByte(c)
		}
		if ok {
			out = append(out, sb.String())
			i += n - 1
		}
	}
	return out
}
