/*******************************************************************************
*
* Copyright 2016 SAP SE
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

package main

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ncw/swift"
	"github.com/sapcc/swift-http-import/pkg/util"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

//Directory describes a directory on the source side which can be scraped.
type Directory struct {
	Job          *Job
	Path         string
	RetryCounter uint
}

//directoryStack is a []Directory that implements LIFO semantics.
type directoryStack []Directory

func (s directoryStack) IsEmpty() bool {
	return len(s) == 0
}

func (s directoryStack) Push(d Directory) directoryStack {
	return append(s, d)
}

func (s directoryStack) Pop() (directoryStack, Directory) {
	l := len(s)
	return s[:l-1], s[l-1]
}

func (s directoryStack) PushBack(d Directory) directoryStack {
	return append([]Directory{d}, s...)
}

//Scraper describes the state of the scraper thread.
type Scraper struct {
	//We use a stack here to ensure that the first job's source is completely
	//scraped, and only then the second job's source is scraped, and so on.
	Stack directoryStack
}

//NewScraper creates a new scraper.
func NewScraper(config *Configuration) *Scraper {
	s := &Scraper{
		Stack: make(directoryStack, 0, len(config.Jobs)),
	}

	//push jobs in *reverse* order so that the first job will be processed first
	for idx := range config.Jobs {
		s.Stack = s.Stack.Push(Directory{
			Job:  config.Jobs[len(config.Jobs)-idx-1],
			Path: "/",
		})
	}

	return s
}

//Done returns true when the scraper has scraped everything.
func (s *Scraper) Done() bool {
	return s.Stack.IsEmpty()
}

//Next scrapes the next directory.
func (s *Scraper) Next() (files []File, countAsFailed bool) {
	if s.Done() {
		return nil, false
	}

	//fetch next directory from stack, list its entries
	var directory Directory
	s.Stack, directory = s.Stack.Pop()
	job := directory.Job //shortcut
	entries, err := job.Source.ListEntries(job, directory.Path)
	//if listing failed, maybe retry later
	if err != nil {
		if directory.RetryCounter >= 2 {
			util.Log(util.LogError, "giving up on %s: %s", err.Location, err.Message)
			return nil, true
		}
		util.Log(util.LogError, "skipping %s for now: %s", err.Location, err.Message)
		directory.RetryCounter++
		s.Stack = s.Stack.PushBack(directory)
		return nil, false
	}

	for _, entryName := range entries {
		pathForMatching := filepath.Join(directory.Path, entryName)
		if strings.HasSuffix(entryName, "/") {
			pathForMatching += "/"
		}

		//ignore explicit excluded patterns
		if job.ExcludeRx != nil && job.ExcludeRx.MatchString(pathForMatching) {
			util.Log(util.LogDebug, "skipping %s: is excluded by `%s`", pathForMatching, job.ExcludeRx.String())
			continue
		}
		//ignore not included patterns
		if job.IncludeRx != nil && !job.IncludeRx.MatchString(pathForMatching) {
			util.Log(util.LogDebug, "skipping %s: is not included by `%s`", pathForMatching, job.IncludeRx.String())
			continue
		}

		//consider the link a directory if it ends with "/"
		if strings.HasSuffix(entryName, "/") {
			s.Stack = s.Stack.Push(Directory{
				Job:  directory.Job,
				Path: filepath.Join(directory.Path, entryName),
			})
		} else {
			file := File{
				Job:  job,
				Path: filepath.Join(directory.Path, entryName),
			}
			//ignore immutable files that have already been transferred
			if job.ImmutableFileRx != nil && job.ImmutableFileRx.MatchString(pathForMatching) {
				if job.IsFileTransferred[file.TargetObjectName()] {
					util.Log(util.LogDebug, "skipping %s: already transferred", pathForMatching)
					continue
				}
			}
			files = append(files, file)
		}
	}

	return files, false
}

//matches scheme prefix (e.g. "http:" or "git+ssh:") at the start of a full URL
//[RFC 3986, 3.1]
var schemeRx = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*:`)

//matches ".." path element
var dotdotRx = regexp.MustCompile(`(?:^|/)\.\.(?:$|/)`)

//ListEntries implements the Location interface.
func (u URLLocation) ListEntries(job *Job, path string) ([]string, *ScrapingError) {
	//get full URL of this subdirectory
	url := URLPathJoin(string(u), path)
	//to get a well-formatted directory listing, the directory URL must have a
	//trailing slash (most web servers automatically redirect from the URL
	//without trailing slash to the URL with trailing slash; others show a
	//slightly different directory listing that we cannot parse correctly)
	if !strings.HasSuffix(url, "/") {
		url += "/"
	}

	util.Log(util.LogDebug, "scraping %s", url)

	//retrieve directory listing
	//TODO: This should send "Accept: text/html", but at least Apache and nginx
	//don't care about the Accept header, anyway, as far as my testing showed.
	response, err := job.HTTPClient.Get(url)
	if err != nil {
		return nil, &ScrapingError{url, "GET failed: " + err.Error()}
	}
	defer response.Body.Close()

	//check that we actually got a directory listing
	if !strings.HasPrefix(response.Status, "2") {
		return nil, &ScrapingError{url, "GET returned status " + response.Status}
	}
	contentType := response.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "text/html") {
		return nil, &ScrapingError{url, "GET returned unexpected Content-Type: " + contentType}
	}

	//find links inside the HTML document
	tokenizer := html.NewTokenizer(response.Body)
	var result []string
	for {
		tokenType := tokenizer.Next()

		switch tokenType {
		case html.ErrorToken:
			//end of document
			return result, nil
		case html.StartTagToken:
			token := tokenizer.Token()

			if token.DataAtom == atom.A {
				//found an <a> tag -- retrieve its href
				var href string
				for _, attr := range token.Attr {
					if attr.Key == "href" {
						href = attr.Val
						break
					}
				}
				if href == "" {
					continue
				}

				//filter external links with full URLs
				if schemeRx.MatchString(href) {
					continue
				}
				//links with trailing slashes are absolute paths as well; either to
				//another server, e.g. "//ajax.googleapis.com/jquery.js", or to the
				//toplevel of this server, e.g. "/static/site.css")
				if strings.HasPrefix(href, "/") {
					continue
				}
				//links with ".." path elements cannot be guaranteed to be pointing to a
				//resource below this directory, so skip them as well (this assumes that
				//the sender did already clean his relative links so that no ".." appears
				//in legitimate downward links)
				if dotdotRx.MatchString(href) {
					continue
				}
				//ignore links with a query part (Apache directory listings use these for
				//adjustable sorting)
				if strings.Contains(href, "?") {
					continue
				}

				result = append(result, href)
			}
		}
	}
}

//ListEntries implements the Location interface.
func (s *SwiftLocation) ListEntries(job *Job, path string) ([]string, *ScrapingError) {
	objectPath := filepath.Join(s.ObjectPrefix, strings.TrimPrefix(path, "/"))
	if objectPath != "" && !strings.HasSuffix(objectPath, "/") {
		objectPath += "/"
	}
	util.Log(util.LogDebug, "listing objects at %s/%s", s.ContainerName, objectPath)

	names, err := s.Connection.ObjectNamesAll(s.ContainerName, &swift.ObjectsOpts{
		Prefix:    objectPath,
		Delimiter: '/',
	})
	if err != nil {
		return nil, &ScrapingError{
			Location: s.ContainerName + "/" + "objectPath",
			Message:  "GET failed: " + err.Error(),
		}
	}

	//ObjectNamesAll returns full names, but we want only the basenames
	for idx, name := range names {
		isDir := strings.HasSuffix(name, "/")
		names[idx] = filepath.Base(name)
		if isDir {
			names[idx] += "/"
		}
	}
	return names, nil
}
