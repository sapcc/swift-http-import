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
	"os"
	"os/signal"
	"sync"
	"syscall"

	"golang.org/x/net/context"
)

//SharedState contains all the stuff shared between all worker threads.
type SharedState struct {
	Configuration
	Context   context.Context
	WaitGroup sync.WaitGroup

	//each of these is only ever written by one thread (and then read by the
	//main thread after waiting on the writing thread), so no additional
	//locking is required for these fields
	DirectoriesScanned uint64
	FilesFound         uint64
	FilesFailed        uint64
	FilesTransferred   uint64
}

//Run starts and orchestrates the various worker threads.
func Run(state *SharedState) (exitCode int) {
	//receive SIGINT/SIGTERM signals
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)

	//setup a context that cancels the workers when one of the signals above is received
	var cancelFunc func()
	state.Context, cancelFunc = context.WithCancel(state.Context)
	defer cancelFunc()
	go func() {
		<-sigs
		cancelFunc()
	}()

	//setup a simple linear pipeline of workers (it should be fairly trivial to
	//scale this out to multiple workers later)
	queue := makeScraperThread(state)
	for i := uint(0); i < state.WorkerCounts.Transfer; i++ {
		makeTransferThread(state, queue)
	}

	//wait for all of them to return
	state.WaitGroup.Wait()

	//send statistics
	Gauge("last_run.dirs_scanned", int64(state.DirectoriesScanned), 1.0)
	Gauge("last_run.files_found", int64(state.FilesFound), 1.0)
	Gauge("last_run.files_transfered", int64(state.FilesTransferred), 1.0)
	Gauge("last_run.files_failed", int64(state.FilesFailed), 1.0)
	if state.FilesFailed > 0 {
		Gauge("last_run.success", 0, 1.0)
		exitCode = 1
	} else {
		Gauge("last_run.success", 1, 1.0)
		exitCode = 0
	}

	//report results
	Log(LogInfo, "%d dirs scanned, %d files found, %d transferred, %d failed",
		state.DirectoriesScanned, state.FilesFound,
		state.FilesTransferred, state.FilesFailed,
	)

	return
}

func makeScraperThread(state *SharedState) <-chan File {
	state.WaitGroup.Add(1)
	out := make(chan File, 10)

	scraper := NewScraper(&state.Configuration)

	go func() {
		defer state.WaitGroup.Done()
		defer close(out)

		var directoriesScanned uint64
		var filesFound uint64

		for {
			//check if state.Context.Done() is closed
			if state.Context.Err() != nil {
				break
			}
			if scraper.Done() {
				break
			}

			for _, file := range scraper.Next() {
				filesFound++
				out <- file
			}
			directoriesScanned++
		}

		//submit statistics to main thread
		state.DirectoriesScanned = directoriesScanned
		state.FilesFound = filesFound
	}()

	return out
}

func makeTransferThread(state *SharedState, in <-chan File) {
	state.WaitGroup.Add(1)
	done := state.Context.Done()

	go func() {
		defer state.WaitGroup.Done()

		//run worker loop
		filesTransferred1, filesFailed1, aborted := transferThreadWorkerLoop(in, done)

		//submit statistics to main thread
		state.FilesFailed = uint64(len(filesFailed1))
		state.FilesTransferred = filesTransferred1
		if aborted || len(filesFailed1) == 0 {
			return
		}

		//if not yet aborted, retry transfer of failed files one more time
		Log(LogInfo, "retrying %d failed file transfers...", len(filesFailed1))
		in2 := make(chan File, len(filesFailed1)+1)
		for _, file := range filesFailed1 {
			in2 <- file
		}
		in2 <- File{Path: ""}
		filesTransferred2, filesFailed2, _ := transferThreadWorkerLoop(in2, done)

		//submit new statistics
		state.FilesFailed = uint64(len(filesFailed2))
		state.FilesTransferred = filesTransferred1 + filesTransferred2
	}()
}

func transferThreadWorkerLoop(in <-chan File, done <-chan struct{}) (filesTransferred uint64, filesFailed []File, aborted bool) {
	for {
		var file File
		select {
		case <-done:
			aborted = true
			return
		case file = <-in:
			if file.Path == "" {
				//input channel is closed and returns zero values
				aborted = false
				return
			}
			switch file.PerformTransfer() {
			case TransferSuccess:
				filesTransferred++
			case TransferSkipped:
				//nothing to count
			case TransferFailed:
				filesFailed = append(filesFailed, file)
			}
		}
	}
}
