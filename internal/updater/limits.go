package updater

import (
	"errors"
	"fmt"
	"io"
	"strings"
)

type extractionLimits struct {
	binaryBytes int64
	totalBytes  int64
	entries     int
}

var releaseExtractionLimits = extractionLimits{binaryBytes: 128 << 20, totalBytes: 256 << 20, entries: 4096}

func extractBinaryWithLimits(content []byte, name string, limits extractionLimits) ([]byte, error) {
	lower := strings.ToLower(name)
	if strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz") {
		return extractFromTarGz(content, limits)
	}
	if strings.HasSuffix(lower, ".zip") {
		return extractFromZip(content, limits)
	}
	if int64(len(content)) > limits.binaryBytes {
		return nil, errors.New("raw binary exceeds extraction size limit")
	}
	if len(content) == 0 {
		return nil, errors.New("release contains an empty binary")
	}
	return content, nil
}

func readBoundedBinary(r io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("binary exceeds extraction size limit")
	}
	if len(data) == 0 {
		return nil, errors.New("archive contains an empty binary")
	}
	return data, nil
}

// Includes discarded tar payloads, padding and extended headers in the budget.
type expansionReader struct {
	reader    io.Reader
	remaining int64
}

func (r *expansionReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if r.remaining == 0 {
		var probe [1]byte
		n, err := r.reader.Read(probe[:])
		if n > 0 {
			return 0, fmt.Errorf("archive exceeds total extraction size limit")
		}
		return 0, err
	}
	if int64(len(p)) > r.remaining {
		p = p[:r.remaining]
	}
	n, err := r.reader.Read(p)
	r.remaining -= int64(n)
	return n, err
}
