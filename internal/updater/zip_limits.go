package updater

import (
	"archive/zip"
	"encoding/binary"
	"errors"
)

// preflightZipDirectory counts central-directory records without constructing
// File objects or copying names/extras. EOCD counts are untrusted (including
// wrapped 16-bit counts); both offsets the Go reader may choose are bounded.
// The standard ZIP reader still performs format/CRC validation after this pass.
func preflightZipDirectory(content []byte, maxEntries int) error {
	end := -1
	minimum := max(0, len(content)-(65*1024))
	for i := len(content) - 22; i >= minimum; i-- {
		if binary.LittleEndian.Uint32(content[i:]) != 0x06054b50 {
			continue
		}
		if int(binary.LittleEndian.Uint16(content[i+20:])) > len(content)-i-22 {
			return zip.ErrFormat
		}
		end = i
		break
	}
	if end < 0 {
		return zip.ErrFormat
	}
	records := uint64(binary.LittleEndian.Uint16(content[end+10:]))
	size := uint64(binary.LittleEndian.Uint32(content[end+12:]))
	offset := uint64(binary.LittleEndian.Uint32(content[end+16:]))
	if records == 0xffff || size == 0xffffffff || offset == 0xffffffff {
		locator := end - 20
		if locator >= 0 && binary.LittleEndian.Uint32(content[locator:]) == 0x07064b50 {
			if binary.LittleEndian.Uint32(content[locator+4:]) != 0 || binary.LittleEndian.Uint32(content[locator+16:]) != 1 {
				return zip.ErrFormat
			}
			position := binary.LittleEndian.Uint64(content[locator+8:])
			if len(content) < 56 || position > uint64(len(content)-56) {
				return zip.ErrFormat
			}
			p := int(position)
			if binary.LittleEndian.Uint32(content[p:]) != 0x06064b50 {
				return zip.ErrFormat
			}
			records = binary.LittleEndian.Uint64(content[p+32:])
			size = binary.LittleEndian.Uint64(content[p+40:])
			offset = binary.LittleEndian.Uint64(content[p+48:])
			end = p
		}
	}
	if records > uint64(maxEntries) {
		return errors.New("archive exceeds entry count limit")
	}
	if size > uint64(end) || offset > uint64(len(content)) {
		return zip.ErrFormat
	}
	start := end - int(size)
	if err := countZipDirectory(content, start, maxEntries); err != nil {
		return err
	}
	// Go normally derives the base offset from the end record. If an archive
	// has a prefix and a valid directory at the declared absolute offset, it
	// may instead use that offset. Preflight it too so neither path bypasses
	// the cap, even when directorySize/base-offset metadata is dishonest.
	if int(offset) != start {
		if err := countZipDirectory(content, int(offset), maxEntries); err != nil {
			return err
		}
	}
	return nil
}

func countZipDirectory(content []byte, position, maxEntries int) error {
	for count := 0; position <= len(content)-4 && binary.LittleEndian.Uint32(content[position:]) == 0x02014b50; count++ {
		if count >= maxEntries {
			return errors.New("archive exceeds entry count limit")
		}
		if position > len(content)-46 {
			return zip.ErrFormat
		}
		header := content[position : position+46]
		length := 46 + int(binary.LittleEndian.Uint16(header[28:])) + int(binary.LittleEndian.Uint16(header[30:])) + int(binary.LittleEndian.Uint16(header[32:]))
		if length > len(content)-position {
			return zip.ErrFormat
		}
		position += length
	}
	return nil
}
