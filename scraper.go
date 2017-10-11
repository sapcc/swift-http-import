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
	"strings"

	"github.com/sapcc/swift-http-import/pkg/objects"
	"github.com/sapcc/swift-http-import/pkg/util"
)

//Directory describes a directory on the source side which can be scraped.
type Directory struct {
	Job          *objects.Job
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
func NewScraper(config *objects.Configuration) *Scraper {
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
func (s *Scraper) Next() (files []objects.File, countAsFailed bool) {
	if s.Done() {
		return nil, false
	}

	//fetch next directory from stack, list its entries
	var directory Directory
	s.Stack, directory = s.Stack.Pop()
	job := directory.Job //shortcut
	entries, err := job.Source.ListEntries(directory.Path)
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

		excludeReason := job.Matcher.Check(pathForMatching)
		if excludeReason != "" {
			util.Log(util.LogDebug, "skipping %s: %s", pathForMatching, excludeReason)
			continue
		}

		//consider the link a directory if it ends with "/"
		if strings.HasSuffix(entryName, "/") {
			s.Stack = s.Stack.Push(Directory{
				Job:  directory.Job,
				Path: filepath.Join(directory.Path, entryName),
			})
		} else {
			files = append(files, objects.File{
				Job:  job,
				Path: filepath.Join(directory.Path, entryName),
			})
		}
	}

	return files, false
}
