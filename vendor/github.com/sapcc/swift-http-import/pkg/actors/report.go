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
	"strconv"
	"time"

	"github.com/cactus/go-statsd-client/statsd"
	"github.com/sapcc/swift-http-import/pkg/objects"
	"github.com/sapcc/swift-http-import/pkg/util"
)

//ReportEvent counts either a directory that was scraped, or a file that was
//found (and maybe transferred). It is consumed by the Report actor.
type ReportEvent struct {
	IsDirectory     bool
	DirectoryFailed bool

	IsFile             bool
	FileTransferResult objects.TransferResult

	IsCleanup            bool
	CleanedUpObjectCount int64
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
	Statsd    objects.StatsdConfiguration
	StartTime time.Time

	ExitCode int
}

//Run implements the Actor interface.
func (r *Report) Run() {
	var (
		directoriesScanned int64
		directoriesFailed  int64
		filesFound         int64
		filesFailed        int64
		filesTransferred   int64
		filesCleanedUp     int64
		statter            statsd.Statter
	)

	//initialize statsd client
	if r.Statsd.HostName != "" {
		var err error
		statter, err = statsd.NewClient(r.Statsd.HostName+":"+strconv.Itoa(r.Statsd.Port), r.Statsd.Prefix)
		// handle any errors
		if err != nil {
			util.Log(util.LogFatal, err.Error())
		}

		// make sure to clean up
		defer statter.Close()
	}

	//collect tally marks until done or aborted
	for mark := range r.Input {
		switch {
		case mark.IsDirectory:
			directoriesScanned++
			if mark.DirectoryFailed {
				directoriesFailed++
			}
		case mark.IsFile:
			filesFound++
			switch mark.FileTransferResult {
			case objects.TransferSuccess:
				filesTransferred++
			case objects.TransferFailed:
				filesFailed++
			}
		case mark.IsCleanup:
			filesCleanedUp += mark.CleanedUpObjectCount
		}
	}

	//send statistics
	var gauge func(string, int64, float32) error
	if statter != nil {
		gauge = statter.Gauge
	} else {
		gauge = func(bucket string, value int64, rate float32) error { return nil }
	}
	gauge("last_run.dirs_scanned", directoriesScanned, 1.0)
	gauge("last_run.files_found", filesFound, 1.0)
	gauge("last_run.files_transfered", filesTransferred, 1.0)
	gauge("last_run.files_failed", filesFailed, 1.0)
	gauge("last_run.files_cleaned_up", filesCleanedUp, 1.0)
	if filesFailed > 0 || directoriesFailed > 0 {
		gauge("last_run.success", 0, 1.0)
		r.ExitCode = 1
	} else {
		gauge("last_run.success", 1, 1.0)
		gauge("last_run.success_timestamp", time.Now().Unix(), 1.0)
		r.ExitCode = 0
	}

	//report results
	util.Log(util.LogInfo, "%d dirs scanned, %d failed",
		directoriesScanned, directoriesFailed,
	)
	util.Log(util.LogInfo, "%d files found, %d transferred, %d failed",
		filesFound, filesTransferred, filesFailed,
	)
	if filesCleanedUp > 0 {
		util.Log(util.LogInfo, "%d old files cleaned up", filesCleanedUp)
	}

	duration := time.Since(r.StartTime)
	gauge("last_run.duration_seconds", int64(duration.Seconds()), 1.0)
	util.Log(util.LogInfo, "finished in %s", duration.String())
}
