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
	"sync"
	"time"

	"github.com/cactus/go-statsd-client/statsd"
	"github.com/sapcc/swift-http-import/pkg/util"
)

//ReportEvent counts either a directory that was scraped, or a file that was
//found (and maybe transferred). It is consumed by the Report actor.
type ReportEvent struct {
	IsDirectory     bool
	DirectoryFailed bool

	IsFile             bool
	FileTransferResult TransferResult
}

//Report is an actor that counts scraped directories and transferred files.
//It emits StatsD metrics (if desired), logs the final report, and decides
//whether to exit with an error status.
//
//Events are read from the `Input` channel until it is closed.
//The `Done` channel can be closed to interrupt the actor.
//If the `Statter` is not nil, statsd metrics will be emitted.
//The `StartTime` is used to measure this run's duration at the end.
//The `ExitCode` can be read after the actor is done.
type Report struct {
	Input     <-chan ReportEvent
	Done      <-chan struct{}
	Statter   statsd.Statter
	StartTime time.Time

	ExitCode int
}

//Run executes this actor.
func (r *Report) Run(wg *sync.WaitGroup) {
	wg.Add(1)
	defer wg.Done()

	var (
		directoriesScanned int64
		directoriesFailed  int64
		filesFound         int64
		filesFailed        int64
		filesTransferred   int64
	)

	//collect tally marks until done or aborted
LOOP:
	for {
		select {
		case <-r.Done:
			break LOOP
		case mark, ok := <-r.Input:
			if !ok {
				break LOOP
			}
			switch {
			case mark.IsDirectory:
				directoriesScanned++
				if mark.DirectoryFailed {
					directoriesFailed++
				}
			case mark.IsFile:
				filesFound++
				switch mark.FileTransferResult {
				case TransferSuccess:
					filesTransferred++
				case TransferFailed:
					filesFailed++
				}
			}
		}
	}

	//send statistics
	var gauge func(string, int64, float32) error
	if r.Statter != nil {
		gauge = r.Statter.Gauge
	} else {
		gauge = func(bucket string, value int64, rate float32) error { return nil }
	}
	gauge("last_run.dirs_scanned", directoriesScanned, 1.0)
	gauge("last_run.files_found", filesFound, 1.0)
	gauge("last_run.files_transfered", filesTransferred, 1.0)
	gauge("last_run.files_failed", filesFailed, 1.0)
	if filesFailed > 0 || directoriesFailed > 0 {
		gauge("last_run.success", 0, 1.0)
		r.ExitCode = 1
	} else {
		gauge("last_run.success", 1, 1.0)
		r.ExitCode = 0
	}

	//report results
	util.Log(util.LogInfo, "%d dirs scanned, %d failed",
		directoriesScanned, directoriesFailed,
	)
	util.Log(util.LogInfo, "%d files found, %d transferred, %d failed",
		filesFound, filesTransferred, filesFailed,
	)

	duration := time.Since(r.StartTime)
	gauge("last_run.duration_seconds", int64(duration.Seconds()), 1.0)
	util.Log(util.LogInfo, "finished in %s", duration.String())
}
