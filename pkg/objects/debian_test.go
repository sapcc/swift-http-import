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

// TestDebianReleasePackagesEntryRx tests the regular expression that is used to
// match an entry in a 'Release' debian control file that represents the path
// for a 'Packages' index file for some architecture for some distribution
// component.
//
// e.g. "main/binary-amd64/Packages.gz"
func TestDebianReleasePackagesEntryRx(t *testing.T) {
	tt := []struct {
		in    string
		match bool
	}{
		{"Packages", false},
		{"Packages.gz", false},
		{"Packages.xz", false},
		{"arch64/Packages", false},
		{"arch64/Packages.gz", false},
		{"arch64/Packages.xz", false},
		{"binary-arch64/Packages", false},
		{"binary-arch64/Packages.gz", false},
		{"binary-arch64/Packages.xz", false},
		{"c0mp0n3t/binary-arch64/Packages", false},
		{"c0mp0n3t/binary-arch64/Packages.gz", false},
		{"c0mp0n3t/binary-arch64/Packages.xz", false},
		{"c0mp0n3t/debian-installer/binary-arch64/Packages.gz", false},
		{"c0mp0n3t/debian-installer/binary-arch64/Packages.xz", false},
		{"component/arch64/Packages", false},
		{"component/arch64/Packages.gz", false},
		{"component/arch64/Packages.xz", false},
		{"component/binary.arch64/Packages.gz", false},
		{"component/binary.arch64/Packages.xz", false},
		{"component/debian.installer/binary-arch64/Packages.gz", false},
		{"component/debian.installer/binary-arch64/Packages.xz", false},
		{"debian-installer/binary-arch64/Packages.gz", false},
		{"debian-installer/binary-arch64/Packages.xz", false},

		{"component/binary-arch64/Packages.gz", true},
		{"component/binary-arch64/Packages.xz", true},
		{"component/debian-installer/binary-arch64/Packages.gz", true},
		{"component/debian-installer/binary-arch64/Packages.xz", true},
	}

	for _, tc := range tt {
		if match := debReleasePackagesEntryRx.MatchString(tc.in); match != tc.match {
			if tc.match {
				t.Errorf("'%s' did not match the regular expression. Was expected to match.\n", tc.in)
			} else {
				t.Errorf("'%s' matched the regular expression. Was expected not to match.\n", tc.in)
			}
		}
	}
}
