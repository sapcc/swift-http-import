/*******************************************************************************
*
* Copyright 2019 SAP SE
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

import "testing"

//TestDebianInReleasePackageEntryPathRx tests the regular expression
//that is used to match an entry in an "InRelease" debian control file that
//represents the path for a 'Packages.gz' file for some architecture for some
//distribution component.
//
//e.g. "main/binary-amd64/Packages.gz"

func TestDebianReleaseIndexEntriesPathRx(t *testing.T) {
	tt := []struct {
		in    string
		match bool
	}{
		{"Packages", false},
		{"Packages.gz", false},
		{"packages.xz", false},
		{"something", false},
		{"something.gz", false},
		{"something.xz", false},
		{"something-arch420", false},
		{"something-arch.gz", false},
		{"something-arch420.xz", false},
		{"something/Packages", false},
		{"something/Packages.gz", false},
		{"something/packages.xz", false},
		{"some/thing", false},
		{"some/thing.gz", false},
		{"some/thing.xz", false},
		{"some/thing-arch420", false},
		{"some/thing-arch.gz", false},
		{"some/thing-arch420.xz", false},
		{"something-arch420/Packages", false},
		{"something-arch/Packages.gz", false},
		{"something-arch420/packages.xz", false},
		{"some-arch420/thing", false},
		{"some-arch/thing.gz", false},
		{"some-arch420/thing.xz", false},
		{"some/arch420/Packages", false},
		{"some/arch/Packages.gz", false},
		{"some/arch420/packages.xz", false},
		{"some/arch420/thing", false},
		{"some/arch/thing.gz", false},
		{"some/arch420/thing.xz", false},
		{"some/other-arch420/thing", false},
		{"some/other-arch/thing.gz", false},
		{"some/other-arch420/thing.xz", false},
		{"some/other-arch/Packages.gz", false},
		{"some/other-arch420/packages.xz", false},
		{"also/some/other-arch420/thing", false},
		{"also/some/other-arch/thing.gz", false},
		{"also/some/other-arch420/thing.xz", false},
		{"also/some/other-arch/Packages.gz", false},
		{"also/some/other-arch420/packages.xz", false},

		{"some/thing-arch/Packages", true},
		{"some/thing-arch420/Packages", true},
		{"some/other-thingy/thing-arch/Packages", true},
		{"some/other-thingy/thing-arch420/Packages", true},
	}

	for _, tc := range tt {
		if match := debReleaseIndexEntriesPathRx.MatchString(tc.in); match != tc.match {
			if tc.match {
				t.Errorf("'%s' did not match the regular expression. Was expected to match.\n", tc.in)
			} else {
				t.Errorf("'%s' matched the regular expression. Was expected not to match.\n", tc.in)
			}
		}
	}
}
