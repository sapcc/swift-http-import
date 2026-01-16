// SPDX-FileCopyrightText: 2017 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package objects

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	. "github.com/majewsky/gg/option"
	"github.com/sapcc/go-bits/regexpext"
)

// Matcher determines if files shall be included or excluded in a transfer.
type Matcher struct {
	ExcludeRx            Option[regexpext.PlainRegexp]
	IncludeRx            Option[regexpext.PlainRegexp]
	ImmutableFileRx      Option[regexpext.PlainRegexp]
	NotOlderThan         *time.Time
	SimplisticComparison *bool
}

// MatchError is returned by the functions on type Matcher.
type MatchError struct {
	Path   string
	Reason string
}

// Error implements the builtin/error interface.
func (e MatchError) Error() string {
	return e.Path + " " + e.Reason
}

// Check checks whether the directory at `path` should be scraped, or whether
// the file at `path` should be transferred.
// If not, a MatchError is returned that contains the concerning `path` and a
// human-readable message describing the exclusion.
//
// If `path` is a directory, `path` must have a trailing slash.
// If `path` is a file, `path` must not have a trailing slash.
//
// For directories, `lastModified` must be nil. For files, `lastModified` may be
// non-nil and will then be checked against `m.NotOlderThan`.
func (m Matcher) Check(path string, lastModified *time.Time) error {
	// The path "/" may be produced by the loop in CheckRecursive(), but it is
	// always considered included.
	if filepath.Clean(path) == "/" {
		return nil
	}

	if lastModified != nil && m.NotOlderThan != nil {
		if m.NotOlderThan.After(*lastModified) {
			return MatchError{Path: path, Reason: "is excluded because of age"}
		}
	}

	if rx, ok := m.ExcludeRx.Unpack(); ok && rx.MatchString(path) {
		return MatchError{Path: path, Reason: fmt.Sprintf("is excluded by `%s`", string(rx))}
	}
	if rx, ok := m.IncludeRx.Unpack(); ok && !rx.MatchString(path) {
		return MatchError{Path: path, Reason: fmt.Sprintf("is not included by `%s`", string(rx))}
	}
	return nil
}

// CheckFile is like CheckRecursive, but uses `spec.Path` and appends a slash if
// `spec.IsDirectory`.
func (m Matcher) CheckFile(spec FileSpec) error {
	if spec.IsDirectory {
		return m.CheckRecursive(spec.Path+"/", nil)
	}
	return m.CheckRecursive(spec.Path, spec.LastModified)
}

// CheckRecursive is like Check(), but also checks each directory along the way
// as well.
//
// For example, CheckRecursive("a/b/c") calls Check("a/"), "Check("a/b/") and
// Check("a/b/c").
func (m Matcher) CheckRecursive(path string, lastModified *time.Time) error {
	steps := strings.SplitAfter(path, "/")
	for i := 1; i < len(steps); i++ {
		err := m.Check(strings.Join(steps[0:i], ""), nil)
		if err != nil {
			return err
		}
	}
	return m.Check(path, lastModified)
}
