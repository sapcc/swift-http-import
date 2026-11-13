// SPDX-FileCopyrightText: 2018 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package objects

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"
)

// AgeSpec is a timestamp that is deserialized from a duration in the format
// "<value> <unit>", e.g. "4 days" or "2 weeks".
type AgeSpec time.Duration

var (
	ageSpecRx    = regexp.MustCompile(`^\s*([0-9]+)\s*(\w+)\s*$`)
	ageSpecUnits = map[string]time.Duration{
		"seconds": time.Second,
		"second":  time.Second,
		"s":       time.Second,

		"minutes": time.Minute,
		"minute":  time.Minute,
		"m":       time.Minute,

		"hours": time.Hour,
		"hour":  time.Hour,
		"h":     time.Hour,

		"days": 24 * time.Hour,
		"day":  24 * time.Hour,
		"d":    24 * time.Hour,

		"weeks": 24 * 7 * time.Hour,
		"week":  24 * 7 * time.Hour,
		"w":     24 * 7 * time.Hour,
	}
)

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (a *AgeSpec) UnmarshalYAML(unmarshal func(any) error) error {
	var input string
	err := unmarshal(&input)
	if err != nil {
		return err
	}

	match := ageSpecRx.FindStringSubmatch(input)
	if match == nil {
		return fmt.Errorf(`expected age specification in the format "<value> <unit>", e.g. "2h" or "4 days", got %q instead`, input)
	}

	count, err := strconv.ParseUint(match[1], 10, 16)
	if err != nil {
		return err
	}

	unit, ok := ageSpecUnits[match[2]]
	if !ok {
		return fmt.Errorf("unknown unit %q", match[2])
	}

	*a = AgeSpec(unit * time.Duration(count))
	return nil
}

var (
	gzipMagicNumber = []byte{0x1f, 0x8b, 0x08}
	xzMagicNumber   = []byte{0xfd, 0x37, 0x7a, 0x58, 0x5a, 0x00}
	zstdMagicNumber = []byte{0x28, 0xb5, 0x2f, 0xfd}
)

// decompressFileTransparently returns a reader for `buf` that uncompresses the
// data if it is compressed in a known format (GZip, XZ, zstd).
func decompressFileTransparently(buf []byte) (r io.Reader, err error) {
	r = bytes.NewReader(buf)
	switch {
	case bytes.HasPrefix(buf, gzipMagicNumber):
		r, err = gzip.NewReader(r)
		if err != nil {
			return nil, fmt.Errorf("while decompressing GZip archive: %w", err)
		}
	case bytes.HasPrefix(buf, xzMagicNumber):
		r, err = xz.NewReader(r)
		if err != nil {
			return nil, fmt.Errorf("while decompressing XZ archive: %w", err)
		}
	case bytes.HasPrefix(buf, zstdMagicNumber):
		r, err = zstd.NewReader(r)
		if err != nil {
			return nil, fmt.Errorf("while decompressing zstd archive: %w", err)
		}
	}
	return r, nil
}
