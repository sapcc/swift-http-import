// SPDX-FileCopyrightText: 2018 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package objects

import (
	"bytes"
	"compress/gzip"
	"errors"
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

var gzipMagicNumber = []byte{0x1f, 0x8b, 0x08}

// decompressGZipArchive decompresses and returns the contents of a slice of
// gzip compressed bytes.
func decompressGZipArchive(buf []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(buf))
	if err != nil {
		return nil, errors.New("error while decompressing GZip archive: " + err.Error())
	}

	decompBuf, err := io.ReadAll(reader)
	if err != nil {
		return nil, errors.New("error while decompressing GZip archive: " + err.Error())
	}

	return decompBuf, nil
}

var xzMagicNumber = []byte{0xfd, 0x37, 0x7a, 0x58, 0x5a, 0x00}

// decompressXZArchive decompresses and returns the contents of a slice of xz
// compressed bytes.
func decompressXZArchive(buf []byte) ([]byte, error) {
	reader, err := xz.NewReader(bytes.NewReader(buf))
	if err != nil {
		return nil, errors.New("error while decompressing XZ archive: " + err.Error())
	}

	decompBuf, err := io.ReadAll(reader)
	if err != nil {
		return nil, errors.New("error while decompressing XZ archive: " + err.Error())
	}

	return decompBuf, nil
}

var zstdMagicNumber = []byte{0x28, 0xb5, 0x2f, 0xfd}

// decompressZSTDArchive decompresses and returns the contents of a slice of
// zstd compressed bytes.
func decompressZSTDArchive(buf []byte) ([]byte, error) {
	reader, err := zstd.NewReader(bytes.NewReader(buf))
	if err != nil {
		return nil, errors.New("error while decompressing zstd archive: " + err.Error())
	}

	decompBuf, err := io.ReadAll(reader)
	if err != nil {
		return nil, errors.New("error while decompressing zstd archive: " + err.Error())
	}

	return decompBuf, nil
}
