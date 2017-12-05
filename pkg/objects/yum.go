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
	"bytes"
	"compress/gzip"
	"encoding/xml"
	"io"
	"io/ioutil"
	"net/http"
)

//YumSource is a URLSource containing a Yum repository. This type reuses the
//Validate() and Connect() logic of URLSource, but adds a custom scraping
//implementation that reads the Yum repository metadata instead of relying on
//directory listings.
type YumSource URLSource

//Validate implements the Source interface.
func (s *YumSource) Validate(name string) []error {
	return (*URLSource)(s).Validate(name)
}

//Connect implements the Source interface.
func (s *YumSource) Connect() error {
	return (*URLSource)(s).Connect()
}

//ListEntries implements the Source interface.
func (s *YumSource) ListEntries(directoryPath string) ([]FileSpec, *ListEntriesError) {
	return nil, &ListEntriesError{
		Location: (*URLSource)(s).getURLForPath(directoryPath).String(),
		Message:  "ListEntries is not implemented for YumSource",
	}
}

//GetFile implements the Source interface.
func (s *YumSource) GetFile(directoryPath string, targetState FileState) (body io.ReadCloser, sourceState FileState, err error) {
	return (*URLSource)(s).GetFile(directoryPath, targetState)
}

//ListAllFiles implements the Source interface.
func (s *YumSource) ListAllFiles() ([]FileSpec, *ListEntriesError) {
	repomdPath := "repodata/repomd.xml"
	cache := make(map[string]FileSpec)

	//parse repomd.xml to find paths of all other metadata files
	var repomd struct {
		Entries []struct {
			Type     string `xml:"type,attr"`
			Location struct {
				Href string `xml:"href,attr"`
			} `xml:"location"`
		} `xml:"data"`
	}
	repomdURL, lerr := s.downloadAndParseXML(repomdPath, &repomd, cache)
	if lerr != nil {
		return nil, lerr
	}

	//note metadata files for transfer
	hrefsByType := make(map[string]string)
	var allFiles []string
	for _, entry := range repomd.Entries {
		allFiles = append(allFiles, entry.Location.Href)
		hrefsByType[entry.Type] = entry.Location.Href
	}

	//parse primary.xml.gz to find paths of RPMs
	href, exists := hrefsByType["primary"]
	if !exists {
		return nil, &ListEntriesError{
			Location: repomdURL,
			Message:  "cannot find link to primary.xml.gz in repomd.xml",
		}
	}
	var primary struct {
		Packages []struct {
			Location struct {
				Href string `xml:"href,attr"`
			} `xml:"location"`
		} `xml:"package"`
	}
	_, lerr = s.downloadAndParseXML(href, &primary, cache)
	if lerr != nil {
		return nil, lerr
	}
	for _, pkg := range primary.Packages {
		allFiles = append(allFiles, pkg.Location.Href)
	}

	//parse prestodelta.xml.gz (if present) to find paths of DRPMs
	href, exists = hrefsByType["prestodelta"]
	if exists {
		var prestodelta struct {
			Packages []struct {
				Delta struct {
					Href string `xml:"filename"`
				} `xml:"delta"`
			} `xml:"newpackage"`
		}
		_, lerr = s.downloadAndParseXML(href, &prestodelta, cache)
		if lerr != nil {
			return nil, lerr
		}
		for _, pkg := range prestodelta.Packages {
			allFiles = append(allFiles, pkg.Delta.Href)
		}
	}

	//transfer repomd.xml at the very end, when everything else has already been
	//uploaded (to avoid situations where a client might see repository metadata
	//without being able to see the referenced packages)
	allFiles = append(allFiles, repomdPath)

	//for files that were already downloaded, pass the contents and HTTP headers
	//into the transfer phase to avoid double download
	//
	//This also ensures that the transferred set of packages is consistent with
	//the transferred repo metadata. If we were to download repomd.xml et al
	//again during the transfer step, there is a chance that new metadata has
	//been uploaded to the source in the meantime. In this case, we would be
	//missing the packages referenced only in the new metadata.
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

//Helper function for YumSource.ListAllFiles().
func (s *YumSource) downloadAndParseXML(path string, data interface{}, cache map[string]FileSpec) (uri string, e *ListEntriesError) {
	buf, uri, lerr := s.getFileContents(path, cache)
	if lerr != nil {
		return uri, lerr
	}

	//if `buf` has the magic number for GZip, decompress before parsing as XML
	if bytes.HasPrefix(buf, []byte{0x1f, 0x8b, 0x08}) {
		reader, err := gzip.NewReader(bytes.NewReader(buf))
		if err == nil {
			buf, err = ioutil.ReadAll(reader)
		}
		if err != nil {
			return uri, &ListEntriesError{
				Location: uri,
				Message:  "error while decompressing GZip archive: " + err.Error(),
			}
		}
	}

	err := xml.Unmarshal(buf, data)
	if err != nil {
		return uri, &ListEntriesError{
			Location: uri,
			Message:  "error while parsing XML: " + err.Error(),
		}
	}

	return uri, nil
}

//Helper function for YumSource.ListAllFiles().
func (s *YumSource) getFileContents(path string, cache map[string]FileSpec) (contents []byte, uri string, e *ListEntriesError) {
	u := (*URLSource)(s)
	uri = u.getURLForPath(path).String()

	req, err := http.NewRequest("GET", uri, nil)
	if err != nil {
		return nil, uri, &ListEntriesError{uri, "GET failed: " + err.Error()}
	}

	resp, err := u.HTTPClient.Do(req)
	if err != nil {
		return nil, uri, &ListEntriesError{uri, "GET failed: " + err.Error()}
	}
	defer resp.Body.Close()

	result, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, uri, &ListEntriesError{uri, "GET failed: " + err.Error()}
	}

	cache[path] = FileSpec{
		Path:     path,
		Contents: result,
		Headers:  resp.Header,
	}

	return result, uri, nil
}
