/*******************************************************************************
*
* Copyright 2017 SAP SE
*
* Licensed under the Apache License, Version 2.0 (the "License");
* you may not use this file except in compliance with the License.
* You should have received a copy of the License along with this
* program. If not, you may obtain a copy of the License at
*
*     http://www.apache.org/licenses/LICENSE-2.0
*
* Unless required by applicable law or agreed to in writing, software
* distributed under the License is distributed on an "AS IS" BASIS,
* WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
* See the License for the specific language governing permissions and
* limitations under the License.
*
*******************************************************************************/

package objects

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

//Matcher determines if files shall be included or excluded in a transfer.
type Matcher struct {
	ExcludeRx       *regexp.Regexp //pointers because nil signifies absence
	IncludeRx       *regexp.Regexp
	ImmutableFileRx *regexp.Regexp
}

//MatchError is returned by the functions on type Matcher.
type MatchError struct {
	Path   string
	Reason string
}

//Error implements the builtin/error interface.
func (e MatchError) Error() string {
	return e.Path + " " + e.Reason
}

//Check checks whether the directory at `path` should be scraped, or
//whether the file at `path` should be transferred. If so, an empty string is
//returned. If not, a non-empty string is returned that contains a
//human-readable message why the file is excluded from the transfer.
//
//If `path` is a directory, `path` must have a trailing slash.
//If `path` is a file, `path` must not have a trailing slash.
func (m Matcher) Check(path string) error {
	//The path "/" may be produced by the loop in CheckRecursive(), but it is
	//always considered included.
	if filepath.Clean(path) == "/" {
		return nil
	}
	if m.ExcludeRx != nil && m.ExcludeRx.MatchString(path) {
		return MatchError{Path: path, Reason: fmt.Sprintf("is excluded by `%s`", m.ExcludeRx.String())}
	}
	if m.IncludeRx != nil && !m.IncludeRx.MatchString(path) {
		return MatchError{Path: path, Reason: fmt.Sprintf("is not included by `%s`", m.IncludeRx.String())}
	}
	return nil
}

//CheckFile is like CheckRecursive, but uses `spec.Path` and appends a slash if
//`spec.IsDirectory`.
func (m Matcher) CheckFile(spec FileSpec) error {
	if spec.IsDirectory {
		return m.CheckRecursive(spec.Path + "/")
	}
	return m.CheckRecursive(spec.Path)
}

//CheckRecursive is like Check(), but also checks each directory along the way
//as well.
//
//For example, CheckRecursive("a/b/c") calls Check("a/"), "Check("a/b/") and
//Check("a/b/c").
func (m Matcher) CheckRecursive(path string) error {
	steps := strings.Split(filepath.Clean(path), "/")
	for i := 1; i < len(steps); i++ {
		result := m.Check(filepath.Join(steps[0:i]...) + "/")
		if result != nil {
			return result
		}
	}
	return m.Check(path)
}
