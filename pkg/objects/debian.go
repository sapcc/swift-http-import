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
	"regexp"
	"strings"

	"github.com/majewsky/schwift"
	"pault.ag/go/debian/control"
)

var debReleaseIndexEntriesPathRx = regexp.MustCompile(`^[a-zA-Z]+\/.*-([a-zA-Z0-9]+)\/Packages$`)

//DebianSource is a URLSource containing a Debian repository. This type reuses
//the Validate() and Connect() logic of URLSource, but adds a custom scraping
//implementation that reads the Debian repository metadata instead of relying
//on directory listings.
type DebianSource struct {
	//options from config file
	URLString     string   `yaml:"url"`
	Distributions []string `yaml:"dist"`
	Architectures []string `yaml:"arch"`
	//compiled configuration
	urlSource *URLSource `yaml:"-"`
}

//Validate implements the Source interface.
func (s *DebianSource) Validate(name string) []error {
	s.urlSource = &URLSource{
		URLString: s.URLString,
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
	cache := make(map[string]FileSpec)

	//since package and source files for different distributions are kept in
	//the common '$REPO_ROOT/pool' directory therefore a record is kept of
	//unique files in order to avoid duplicates in the allFiles slice
	var allFiles []string
	uniqueFiles := make(map[string]bool)

	//index files for different distributions as specified in the config file
	for _, distName := range s.Distributions {
		distRootPath := "dists/" + distName + "/"
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
	//parse 'inRelease' file to find paths of other control files
	releasePath := distRootPath + "InRelease"

	var release struct {
		Entries []control.SHA256FileHash `control:"SHA256" delim:"\n" strip:"\n\r\t "`
	}

	_, lerr := s.downloadAndParseDCF(releasePath, &release, cache)
	if lerr != nil {
		//some older distros only have the legacy 'Release' file
		releasePath = distRootPath + "Release"
		_, lerr = s.downloadAndParseDCF(releasePath, &release, cache)
		if lerr != nil {
			return nil, lerr
		}
	}

	var distFiles []string
	var sourceIndices []string
	packageIndicesByArch := make(map[string][]string)

	for _, entry := range release.Entries {
		//entry.Filename is relative to distRootPath therefore
		fileName := distRootPath + entry.Filename

		//note control files for transfer
		distFiles = append(distFiles, fileName)

		//note source indices for source file indexing
		if strings.HasSuffix(entry.Filename, "source/Sources") {
			sourceIndices = append(sourceIndices, fileName)
		}

		//note package indices for package file indexing (as per the config file)
		if match := debReleaseIndexEntriesPathRx.MatchString(entry.Filename); match {
			//matchList = ["full match", "architecture"]
			matchList := debReleaseIndexEntriesPathRx.FindStringSubmatch(entry.Filename)
			if len(s.Architectures) != 0 {
				for _, arch := range s.Architectures {
					if matchList[1] == arch {
						packageIndicesByArch[matchList[1]] = append(packageIndicesByArch[matchList[1]], fileName)
					}
				}
			} else {
				packageIndicesByArch[matchList[1]] = append(packageIndicesByArch[matchList[1]], fileName)

			}
		}
	}

	//parse 'Packages' file to find paths for package files (.deb)
	type packageIndex []struct {
		Filename string `control:"Filename"`
	}

	for _, pkgIndexList := range packageIndicesByArch {
		for _, pkgIndexPath := range pkgIndexList {
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
	}

	//parse 'Packages' file to find paths for package files (.deb)
	type sourceIndex []struct {
		Directory string                `control:"Directory"`
		Files     []control.MD5FileHash `control:"Files" delim:"\n" strip:"\n\r\t "`
	}

	for _, srcIndexPath := range sourceIndices {
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
				distFiles = append(distFiles, src.Directory+file.Filename)
			}
		}
	}

	//transfer 'Release' file at the very end, when everything else has already been
	//uploaded (to avoid situations where a client might see repository metadata
	//without being able to see the referenced packages)
	distFiles = append(distFiles, releasePath)

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
