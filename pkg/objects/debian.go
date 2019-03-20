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

import (
	"bytes"
	"io"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/majewsky/schwift"
	"pault.ag/go/debian/control"
)

var (
	//'Contents' indices
	//Reference:
	//  For '$COMP/Contents-$SARCH.(gz|xz)' or '$COMP/Contents-udeb-$SARCH.(gz|xz)',
	//  where '$SARCH' is either a binary architecture or the
	//  pseudo-architecture "source" that represents source packages.
	//
	//  matchList[1] = "$COMP"
	//  matchList[4] = "$SARCH"
	debReleaseContentsEntryRx = regexp.MustCompile(`^(([a-zA-Z]+)\/)?Contents\-(udeb\-)?([a-zA-Z0-9]+)(\.gz|\.xz)$`)

	//'dep11' files
	//Reference:
	//  For '$COMP/dep11/icons-$DIMENSIONS.tar.gz', where $DIMENSIONS are
	//  in pixels (e.g. 64x64) or '$COMP/dep11/Components-$ARCH.yml.gz'.
	//
	//  matchList[1] = "$COMP"
	//  matchList[2] = "filename with extension"
	//  matchList[4] = "icon dimensions"
	//  matchList[6] = "$ARCH"
	debReleaseDep11EntryRx = regexp.MustCompile(`^([a-zA-Z]+)\/dep11\/((icons\-([0-9]+x[0-9]+)\.tar)|(Components\-([a-zA-Z0-9]+)\.yml))(\.xz|\.gz)$`)

	//'Packages' indices
	//Reference:
	//  For '$COMP/binary-$ARCH/Packages.(gz|xz)' or
	//  '$COMP/debian-installer/binary-$ARCH/Packages.(gz|xz)'.
	//
	//  matchList[1] = "$COMP"
	//  matchList[3] = "$ARCH"
	debReleasePackagesEntryRx = regexp.MustCompile(`^([a-zA-Z]+)\/(debian\-installer\/)?binary\-([a-zA-Z0-9]+)\/Packages(\.gz|\.xz)$`)

	//'Translations' indices
	//Reference:
	//  For '$COMP/i18n/Translation-$LANG(.gz|.xz)'.
	//
	//  matchList[1] = "$COMP"
	debReleaseTranslationEntryRx = regexp.MustCompile(`^([a-zA-Z]+)\/i18n\/((Index)|(Translation\-[a-zA-Z0-9_-]+(\.gz|\.xz)))$`)
)

//DebianSource is a URLSource containing a Debian repository. This type reuses
//the Validate() and Connect() logic of URLSource, but adds a custom scraping
//implementation that reads the Debian repository metadata instead of relying
//on directory listings.
type DebianSource struct {
	//options from config file
	URLString                string   `yaml:"url"`
	ClientCertificatePath    string   `yaml:"cert"`
	ClientCertificateKeyPath string   `yaml:"key"`
	ServerCAPath             string   `yaml:"ca"`
	Distributions            []string `yaml:"dist"`
	Architectures            []string `yaml:"arch"`
	//compiled configuration
	urlSource *URLSource `yaml:"-"`
}

//Validate implements the Source interface.
func (s *DebianSource) Validate(name string) []error {
	s.urlSource = &URLSource{
		URLString:                s.URLString,
		ClientCertificatePath:    s.ClientCertificatePath,
		ClientCertificateKeyPath: s.ClientCertificateKeyPath,
		ServerCAPath:             s.ServerCAPath,
	}
	return s.urlSource.Validate(name)
}

//Connect implements the Source interface.
func (s *DebianSource) Connect() error {
	return s.urlSource.Connect()
}

//ListEntries implements the Source interface.
func (s *DebianSource) ListEntries(directoryPath string) ([]FileSpec, *ListEntriesError) {
	return nil, &ListEntriesError{
		Location: s.urlSource.getURLForPath(directoryPath).String(),
		Message:  "ListEntries is not implemented for DebianSource",
	}
}

//GetFile implements the Source interface.
func (s *DebianSource) GetFile(directoryPath string, requestHeaders schwift.ObjectHeaders) (body io.ReadCloser, sourceState FileState, err error) {
	return s.urlSource.GetFile(directoryPath, requestHeaders)
}

//ListAllFiles implements the Source interface.
func (s *DebianSource) ListAllFiles() ([]FileSpec, *ListEntriesError) {
	if len(s.Distributions) == 0 {
		return nil, &ListEntriesError{
			Location: s.URLString,
			Message:  "no distributions specified in the config file",
		}
	}

	cache := make(map[string]FileSpec)

	//since package and source files for different distributions are kept in
	//the common '$REPO_ROOT/pool' directory therefore a record is kept of
	//unique files in order to avoid duplicates in the allFiles slice
	var allFiles []string
	uniqueFiles := make(map[string]bool)

	//index files for different distributions as specified in the config file
	for _, distName := range s.Distributions {
		distRootPath := filepath.Join("dists", distName)
		distFiles, lerr := s.ListDistFiles(distRootPath, cache)
		if lerr != nil {
			return nil, lerr
		}

		for _, file := range distFiles {
			if duplicate := uniqueFiles[file]; duplicate {
				continue //skip this duplicate
			}

			allFiles = append(allFiles, file)
			uniqueFiles[file] = true
		}
	}

	//for files that were already downloaded, pass the contents and HTTP headers
	//into the transfer phase to avoid double download
	result := make([]FileSpec, len(allFiles))
	for idx, path := range allFiles {
		var exists bool
		result[idx], exists = cache[path]
		if !exists {
			result[idx] = FileSpec{Path: path}
		}
	}

	return result, nil
}

//Helper function for DebianSource.ListAllFiles().
func (s *DebianSource) ListDistFiles(distRootPath string, cache map[string]FileSpec) ([]string, *ListEntriesError) {
	var distFiles []string

	//parse 'inRelease' file to find paths of other control files
	releasePath := filepath.Join(distRootPath, "InRelease")

	var release struct {
		Architectures []string                 `control:"Architectures" delim:" " strip:" "`
		Components    []string                 `control:"Components" delim:" " strip:" "`
		Entries       []control.SHA256FileHash `control:"SHA256" delim:"\n" strip:"\n\r\t "`
		AcquireByHash bool                     `control:"Acquire-By-Hash"`
	}

	_, lerr := s.downloadAndParseDCF(releasePath, &release, cache)
	if lerr != nil {
		//some older distros only have the legacy 'Release' file
		releasePath = filepath.Join(distRootPath, "Release")
		_, lerr = s.downloadAndParseDCF(releasePath, &release, cache)
		if lerr != nil {
			return nil, lerr
		}
	}

	//the architectures that we are interested in
	architectures := release.Architectures
	if len(s.Architectures) != 0 {
		architectures = s.Architectures
	}

	//some repos support the optional 'by-hash' locations as an alternative to
	//the canonical location (and name) of an index file
	//note 'by-hash/SHA256' files for transfer
	if release.AcquireByHash {
		//get a file listing for '$DIST_ROOT/by-hash/'
		entries, lerr := s.recursivelyListEntries(filepath.Join(distRootPath, "by-hash"))
		if lerr != nil {
			return nil, lerr
		}
		distFiles = append(distFiles, entries...)

		for _, component := range release.Components {
			//get a file listing for each '$DIST_ROOT/$COMPONENT/binary-$ARCH/by-hash/'
			for _, arch := range architectures {
				entries, lerr := s.recursivelyListEntries(filepath.Join(distRootPath, component, "binary-"+arch, "by-hash"))
				if lerr != nil {
					return nil, lerr
				}
				distFiles = append(distFiles, entries...)

				//get a file listing for each '$DIST_ROOT/$COMPONENT/debian-installer/binary-$ARCH/by-hash/'
				entries, lerr = s.recursivelyListEntries(filepath.Join(distRootPath, component, "debian-installer", "binary-"+arch, "by-hash"))
				if lerr != nil {
					return nil, lerr
				}
				distFiles = append(distFiles, entries...)
			}

			//get a file listing for each '$DIST_ROOT/$COMPONENT/dep11/by-hash/'
			entries, lerr = s.recursivelyListEntries(filepath.Join(distRootPath, component, "dep11", "by-hash"))
			if lerr != nil {
				return nil, lerr
			}
			distFiles = append(distFiles, entries...)

			//get a file listing for each '$DIST_ROOT/$COMPONENT/i18n/by-hash/'
			entries, lerr = s.recursivelyListEntries(filepath.Join(distRootPath, component, "i18n", "by-hash"))
			if lerr != nil {
				return nil, lerr
			}
			distFiles = append(distFiles, entries...)

			//get a file listing for each '$DIST_ROOT/$COMPONENT/source/by-hash/'
			entries, lerr = s.recursivelyListEntries(filepath.Join(distRootPath, component, "source", "by-hash"))
			if lerr != nil {
				return nil, lerr
			}
			distFiles = append(distFiles, entries...)
		}
	}

	//some repos offer multiple compression types for the same 'Sources' and
	//'Packages' indices. These maps contain the indices with out their file
	//extension. This allows us to choose a compression type at the time of
	//parsing and avoids parsing the same index multiple times.
	sourceIndices := make(map[string]bool)
	packageIndices := make(map[string]bool)

	//note control files for transfer
	for _, entry := range release.Entries {
		//entry.Filename is relative to distRootPath therefore
		fileName := filepath.Join(distRootPath, entry.Filename)

		//note architecture independant files
		switch {
		//note all 'Sources' indices (as they are architecture independent)
		case strings.HasSuffix(entry.Filename, "Sources.gz") || strings.HasSuffix(entry.Filename, "Sources.xz"):
			distFiles = append(distFiles, fileName)

			if exists := sourceIndices[stripFileExtension(fileName)]; !exists {
				sourceIndices[stripFileExtension(fileName)] = true
			}

		//note all 'Translation' indices
		case debReleaseTranslationEntryRx.MatchString(entry.Filename):
			distFiles = append(distFiles, fileName)
		}

		//note architecture specific files
		for _, arch := range architectures {
			//note 'Contents' indices
			switch {
			case debReleaseContentsEntryRx.MatchString(entry.Filename):
				matchList := debReleaseContentsEntryRx.FindStringSubmatch(entry.Filename)
				if matchList[4] == arch {
					distFiles = append(distFiles, fileName)
				}

			//note 'dep11' files
			case debReleaseDep11EntryRx.MatchString(entry.Filename):
				matchList := debReleaseDep11EntryRx.FindStringSubmatch(entry.Filename)
				if matchList[6] != "" {
					if matchList[6] == arch {
						//'dep11' components files
						distFiles = append(distFiles, fileName)
					}
				} else {
					//'dep11' icon files
					distFiles = append(distFiles, fileName)
				}

			//note 'Packages' indices
			case debReleasePackagesEntryRx.MatchString(entry.Filename):
				matchList := debReleasePackagesEntryRx.FindStringSubmatch(entry.Filename)
				if matchList[3] == arch {
					distFiles = append(distFiles, fileName)

					if exists := packageIndices[stripFileExtension(fileName)]; !exists {
						packageIndices[stripFileExtension(fileName)] = true
					}
				}
			}
		}
	}

	//parse 'Packages' file to find paths for package files (.deb)
	type packageIndex []struct {
		Filename string `control:"Filename"`
	}

	for pkgIndexPath := range packageIndices {
		var tmp packageIndex
		//get package index from 'Packages.xz'
		_, lerr := s.downloadAndParseDCF(pkgIndexPath+".xz", &tmp, cache)
		if lerr != nil {
			//some older distros only have 'Packages.gz'
			_, lerr = s.downloadAndParseDCF(pkgIndexPath+".gz", &tmp, cache)
			if lerr != nil {
				return nil, lerr
			}
		}

		for _, pkg := range tmp {
			distFiles = append(distFiles, pkg.Filename)
		}
	}

	//parse 'Sources' file to find paths for source files (.dsc, .tar.gz, etc.)
	type sourceIndex []struct {
		Directory string                `control:"Directory"`
		Files     []control.MD5FileHash `control:"Files" delim:"\n" strip:"\n\r\t "`
	}

	for srcIndexPath := range sourceIndices {
		var tmp sourceIndex
		//get source index from 'Sources.xz'
		_, lerr := s.downloadAndParseDCF(srcIndexPath+".xz", &tmp, cache)
		if lerr != nil {
			//some older distros only have 'Sources.gz'
			_, lerr = s.downloadAndParseDCF(srcIndexPath+".gz", &tmp, cache)
			if lerr != nil {
				return nil, lerr
			}
		}

		for _, src := range tmp {
			for _, file := range src.Files {
				distFiles = append(distFiles, filepath.Join(src.Directory, file.Filename))
			}
		}
	}

	//transfer 'Release' files at the very end, when everything else has
	//already been uploaded (to avoid situations where a client might see
	//repository metadata without being able to see the referenced packages)
	distFiles = append(distFiles, filepath.Join(distRootPath, "InRelease"))
	distFiles = append(distFiles, filepath.Join(distRootPath, "Release"))
	//'Release' file comes with a detached GPG signature rather than an inline
	//one (as in the case of 'InRelease')
	distFiles = append(distFiles, filepath.Join(distRootPath, "Release.gpg"))

	return distFiles, nil
}

//Helper function for DebianSource.ListAllFiles().
func (s *DebianSource) downloadAndParseDCF(path string, data interface{}, cache map[string]FileSpec) (uri string, e *ListEntriesError) {
	buf, uri, lerr := s.urlSource.getFileContents(path, cache)
	if lerr != nil {
		return uri, lerr
	}

	//if `buf` has the magic number for XZ, decompress before parsing as DCF
	if bytes.HasPrefix(buf, xzMagicNumber) {
		var err error
		buf, err = decompressXZArchive(buf)
		if err != nil {
			return uri, &ListEntriesError{Location: uri, Message: err.Error()}
		}
	}

	//if `buf` has the magic number for GZip, decompress before parsing as DCF
	if bytes.HasPrefix(buf, gzipMagicNumber) {
		var err error
		buf, err = decompressGZipArchive(buf)
		if err != nil {
			return uri, &ListEntriesError{Location: uri, Message: err.Error()}
		}
	}

	err := control.Unmarshal(data, bytes.NewReader(buf))
	if err != nil {
		return uri, &ListEntriesError{
			Location: uri,
			Message:  "error while parsing Debian Control File: " + err.Error(),
		}
	}

	return uri, nil
}

//Helper function for DebianSource.ListAllFiles().
func stripFileExtension(fileName string) string {
	ext := filepath.Ext(fileName)
	if ext == "" {
		return fileName
	}

	return strings.TrimSuffix(fileName, ext)
}

//Helper function for DebianSource.ListAllFiles().
func (s *DebianSource) recursivelyListEntries(path string) ([]string, *ListEntriesError) {
	var files []string

	entries, lerr := s.urlSource.ListEntries(path)
	if lerr != nil {
		return nil, lerr
	}

	for _, entry := range entries {
		if entry.IsDirectory {
			tmpFiles, lerr := s.recursivelyListEntries(entry.Path)
			if lerr != nil {
				return nil, lerr
			}
			files = append(files, tmpFiles...)
		} else {
			files = append(files, entry.Path)
		}
	}

	return files, nil
}
