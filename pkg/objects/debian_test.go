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

//TestDebianReleaseContentsEntryRx tests the regular expression that is used to
//match an entry in a 'Release' debian control file that represents the path
//for a 'Contents' index file for some architecture (also the 'source'
//pseudo-architecture) for some distribution component.
//
//e.g. "main/Contents-udeb-amd64.gz"
func TestDebianReleaseContentsEntryRx(t *testing.T) {
	tt := []struct {
		in    string
		match bool
	}{
		{"Contents-arch64", false},
		{"Contents-source", false},
		{"Contents-udeb-source", false},
		{"Contents.arch64.gz", false},
		{"Contents.source.gz", false},
		{"Contents.udeb-arch64.gz", false},
		{"Contents.udeb-source.xz", false},
		{"Dontents-arch64.gz", false},
		{"Dontents-arch64.xz", false},
		{"Dontents-source.gz", false},
		{"Dontents-source.xz", false},
		{"c0mp0n3t/Contents-arch64.gz", false},
		{"c0mp0n3t/Contents-arch64.xz", false},
		{"c0mp0n3t/Contents-source.gz", false},
		{"c0mp0n3t/Contents-source.xz", false},
		{"c0mp0n3t/Contents-udeb-arch64.gz", false},
		{"c0mp0n3t/Contents-udeb-source.xz", false},
		{"component/Contents-arch64", false},
		{"component/Contents-source", false},
		{"component/Contents-udeb-arch64", false},
		{"component/Contents-udeb-source", false},
		{"component/Dontents-arch64.gz", false},
		{"component/Dontents-arch64.xz", false},
		{"component/Dontents-source.gz", false},
		{"component/Dontents-source.xz", false},
		{"component/Dontents-udeb-arch64.gz", false},
		{"component/Dontents-udeb-source.xz", false},

		{"Contents-arch64.gz", true},
		{"Contents-arch64.xz", true},
		{"Contents-source.gz", true},
		{"Contents-source.xz", true},
		{"Contents-udeb-arch64.gz", true},
		{"Contents-udeb-source.xz", true},
		{"component/Contents-arch64.gz", true},
		{"component/Contents-arch64.xz", true},
		{"component/Contents-source.gz", true},
		{"component/Contents-source.xz", true},
		{"component/Contents-udeb-arch64.gz", true},
		{"component/Contents-udeb-source.xz", true},
	}

	for _, tc := range tt {
		if match := debReleaseContentsEntryRx.MatchString(tc.in); match != tc.match {
			if tc.match {
				t.Errorf("'%s' did not match the regular expression. Was expected to match.\n", tc.in)
			} else {
				t.Errorf("'%s' matched the regular expression. Was expected not to match.\n", tc.in)
			}
		}
	}
}

//TestDebianReleaseTranslationEntryRx tests the regular expression that is used to
//match an entry in a 'Release' debian control file that represents the path
//for a 'dep11' file.
//
//e.g. "multiverse/dep11/Components-amd64.yml.gz"
func TestDebianReleaseDep11EntryRx(t *testing.T) {
	tt := []struct {
		in    string
		match bool
	}{
		{"dep11/Components-ppc64el.yml.gz", false},
		{"icons-128x128.tar.gz", false},
		{"multiverse/dep11/Components-ppc64el.yml", false},
		{"multiverse/dep11/Components-ppc64el.yml.dz", false},
		{"multiverse/dep11/Components-ppc64el.zml.gz", false},
		{"multiverse/dep11/Domponents-ppc64el.yml.gz", false},
		{"multiverse/dep11/favicons-128x128.tar.gz", false},
		{"multiverse/dep11/icons-128x128.bar.gz", false},
		{"multiverse/dep11/icons-128x128.tar", false},
		{"multiverse/dep11/icons-128x128.tar.dz", false},
		{"multiverse/dep11/icons-128y128.tar.gz", false},

		{"multiverse/dep11/Components-ppc64el.yml.gz", true},
		{"multiverse/dep11/Components-ppc64el.yml.xz", true},
		{"multiverse/dep11/icons-128x128.tar.gz", true},
		{"multiverse/dep11/icons-128x128.tar.xz", true},
	}

	for _, tc := range tt {
		if match := debReleaseDep11EntryRx.MatchString(tc.in); match != tc.match {
			if tc.match {
				t.Errorf("'%s' did not match the regular expression. Was expected to match.\n", tc.in)
			} else {
				t.Errorf("'%s' matched the regular expression. Was expected not to match.\n", tc.in)
			}
		}
	}
}

//TestDebianReleaseTranslationEntryRx tests the regular expression that is used to
//match an entry in a 'Release' debian control file that represents the path
//for a 'Translation' file.
//
//e.g. "multiverse/i18n/Translation-de"
func TestDebianReleaseTranslationEntryRx(t *testing.T) {
	tt := []struct {
		in    string
		match bool
	}{
		{"Translation-de", false},
		{"Translation-tr.gz", false},
		{"Translation-uk.gz", false},
		{"component/i17n/Translation-de", false},
		{"component/i17n/Translation-tr.gz", false},
		{"component/i17n/Translation-uk.gz", false},
		{"component/i18n/Dranslation-de", false},
		{"component/i18n/Dranslation-tr.gz", false},
		{"component/i18n/Dranslation-uk.gz", false},
		{"component/i18n/Translation-de", false},
		{"component/i18n/Translation-de.tar", false},
		{"i18n.Translation-de", false},
		{"i18n.Translation-tr.gz", false},
		{"i18n.Translation-uk.gz", false},
		{"i18n/Translation-de", false},
		{"i18n/Translation-tr.gz", false},
		{"i18n/Translation-uk.gz", false},

		{"component/i18n/Translation-tr.gz", true},
		{"component/i18n/Translation-tr.xz", true},
		{"component/i18n/Translation-uk.gz", true},
		{"component/i18n/Translation-uk.xz", true},
	}

	for _, tc := range tt {
		if match := debReleaseTranslationEntryRx.MatchString(tc.in); match != tc.match {
			if tc.match {
				t.Errorf("'%s' did not match the regular expression. Was expected to match.\n", tc.in)
			} else {
				t.Errorf("'%s' matched the regular expression. Was expected not to match.\n", tc.in)
			}
		}
	}
}

//TestDebianReleasePackagesEntryRx tests the regular expression that is used to
//match an entry in a 'Release' debian control file that represents the path
//for a 'Packages' index file for some architecture for some distribution
//component.
//
//e.g. "main/binary-amd64/Packages.gz"
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
