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
	"sync"

	"github.com/sapcc/swift-http-import/pkg/actors"
	"github.com/sapcc/swift-http-import/pkg/objects"
	"github.com/sapcc/swift-http-import/pkg/util"

	"golang.org/x/net/context"
)

//SharedState contains all the stuff shared between all worker threads.
type SharedState struct {
	objects.Configuration
	Context   context.Context
	WaitGroup sync.WaitGroup
	Report    chan<- actors.ReportEvent
}

//Run starts and orchestrates the various worker threads.
func Run(state *SharedState) {
	//setup a simple linear pipeline of workers (it should be fairly trivial to
	//scale this out to multiple workers later)
	queue := makeScraperThread(state)
	for i := uint(0); i < state.WorkerCounts.Transfer; i++ {
		makeTransferThread(state, queue)
	}

	//wait for all of them to return
	state.WaitGroup.Wait()
}

func makeScraperThread(state *SharedState) <-chan objects.File {
	state.WaitGroup.Add(1)
	out := make(chan objects.File, 10)

	scraper := NewScraper(&state.Configuration)

	go func() {
		defer state.WaitGroup.Done()
		defer close(out)

		for {
			//check if state.Context.Done() is closed
			if state.Context.Err() != nil {
				break
			}
			if scraper.Done() {
				break
			}

			files, countAsFailed := scraper.Next()
			for _, file := range files {
				out <- file
			}
			state.Report <- actors.ReportEvent{
				IsDirectory:     true,
				DirectoryFailed: countAsFailed,
			}
		}
	}()

	return out
}

func makeTransferThread(state *SharedState, in <-chan objects.File) {
	state.WaitGroup.Add(1)
	done := state.Context.Done()

	go func() {
		defer state.WaitGroup.Done()

		//main transfer loop - report successful and skipped transfers immediately,
		//but push back failed transfers for later retry
		aborted := false
		var filesToRetry []objects.File
	LOOP:
		for {
			select {
			case <-done:
				aborted = true
				break LOOP
			case file, ok := <-in:
				if !ok {
					break LOOP
				}
				result := file.PerformTransfer()
				if result == objects.TransferFailed {
					filesToRetry = append(filesToRetry, file)
				} else {
					state.Report <- actors.ReportEvent{IsFile: true, FileTransferResult: result}
				}
			}
		}

		//retry transfer of failed files one more time
		if len(filesToRetry) == 0 {
			return
		}
		if !aborted {
			util.Log(util.LogInfo, "retrying %d failed file transfers...", len(filesToRetry))
		}
		for _, file := range filesToRetry {
			result := objects.TransferFailed
			//...but only if we were not aborted (this is checked in every loop
			//iteration because the abort signal (i.e. Ctrl-C) could also happen
			//during this loop)
			if !aborted && state.Context.Err() == nil {
				result = file.PerformTransfer()
			}
			state.Report <- actors.ReportEvent{IsFile: true, FileTransferResult: result}
		}

	}()
}
