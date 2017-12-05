/*******************************************************************************
*
* Copyright 2016-2017 SAP SE
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

package actors

import (
	"github.com/sapcc/swift-http-import/pkg/objects"
	"github.com/sapcc/swift-http-import/pkg/util"
	"golang.org/x/net/context"
)

//Scraper is an actor that reads directory listings on the source side to
//enumerate all files that need to be transferred.
//
//Scraping starts from the root directories of each job in the `Jobs` list.
//For each input file, a File struct is sent into the `Output` channel.
//For each directory, a report is sent into the `Report` channel.
type Scraper struct {
	Context context.Context
	Jobs    []*objects.Job
	Output  chan<- objects.File
	Report  chan<- ReportEvent
}

//Run implements the Actor interface.
func (s *Scraper) Run() {
	//push jobs in *reverse* order so that the first job will be processed first
	stack := make(directoryStack, 0, len(s.Jobs))
	for idx := range s.Jobs {
		stack = stack.Push(objects.Directory{
			Job:  s.Jobs[len(s.Jobs)-idx-1],
			Path: "/",
		})
	}

	for !stack.IsEmpty() {
		//check if state.Context.Done() is closed
		if s.Context.Err() != nil {
			break
		}

		//fetch next directory from stack, list its entries
		var directory objects.Directory
		stack, directory = stack.Pop()
		job := directory.Job //shortcut

		//at the top level, try ListAllFiles if supported by job.Source
		var (
			entries []objects.FileSpec
			err     *objects.ListEntriesError
		)
		if directory.Path == "/" {
			entries, err = job.Source.ListAllFiles()
			if err == objects.ErrListAllFilesNotSupported {
				entries, err = job.Source.ListEntries(directory.Path)
			}
		} else {
			entries, err = job.Source.ListEntries(directory.Path)
		}

		//if listing failed, maybe retry later
		if err != nil {
			if directory.RetryCounter >= 2 {
				util.Log(util.LogError, "giving up on %s: %s", err.Location, err.Message)
				s.Report <- ReportEvent{IsDirectory: true, DirectoryFailed: true}
				continue
			}
			util.Log(util.LogError, "skipping %s for now: %s", err.Location, err.Message)
			directory.RetryCounter++
			stack = stack.PushBack(directory)
			continue
		}

		//handle each file/subdirectory that was found
		for _, entry := range entries {
			excludeReason := job.Matcher.CheckFile(entry)
			if excludeReason != "" {
				util.Log(util.LogDebug, "skipping %s: %s", entry.Path, excludeReason)
				continue
			}

			if entry.IsDirectory {
				stack = stack.Push(objects.Directory{
					Job:  directory.Job,
					Path: entry.Path,
				})
			} else {
				s.Output <- objects.File{
					Job:  job,
					Spec: entry,
				}
			}
		}

		//report that a directory was successfully scraped
		s.Report <- ReportEvent{IsDirectory: true}
	}

	//signal to consumers that we're done
	close(s.Output)
}

//directoryStack is a []objects.Directory that implements LIFO semantics.
type directoryStack []objects.Directory

func (s directoryStack) IsEmpty() bool {
	return len(s) == 0
}

func (s directoryStack) Push(d objects.Directory) directoryStack {
	return append(s, d)
}

func (s directoryStack) Pop() (directoryStack, objects.Directory) {
	l := len(s)
	return s[:l-1], s[l-1]
}

func (s directoryStack) PushBack(d objects.Directory) directoryStack {
	return append([]objects.Directory{d}, s...)
}
