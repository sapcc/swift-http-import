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
	"github.com/sapcc/go-bits/logg"
	"github.com/sapcc/swift-http-import/pkg/objects"
)

//ReportEvent counts either a directory that was scraped, or a file that was
//found (and maybe transferred). It is consumed by the Report actor.
type ReportEvent struct {
	IsJob      bool
	JobSkipped bool

	IsDirectory     bool
	DirectoryFailed bool

	IsFile             bool
	FileTransferResult objects.TransferResult
	FileTransferBytes  int64

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
	Stats     Stats

	ExitCode int
}

//Stats contains the report statistics
type Stats struct {
	DirectoriesScanned int64
	DirectoriesFailed  int64
	FilesFound         int64
	FilesFailed        int64
	FilesTransferred   int64
	FilesCleanedUp     int64
	BytesTransferred   int64
	JobsSkipped        int64
}

//Run implements the Actor interface.
func (r *Report) Run() {
	var statter statsd.Statter

	//initialize statsd client
	if r.Statsd.HostName != "" {
		var err error
		statter, err = statsd.NewClient(r.Statsd.HostName+":"+strconv.Itoa(r.Statsd.Port), r.Statsd.Prefix)
		// handle any errors
		if err != nil {
			logg.Fatal(err.Error())
		}

		// make sure to clean up
		defer statter.Close()
	}

	//collect tally marks until done or aborted
	for mark := range r.Input {
		switch {
		case mark.IsDirectory:
			r.Stats.DirectoriesScanned++
			if mark.DirectoryFailed {
				r.Stats.DirectoriesFailed++
			}
		case mark.IsFile:
			r.Stats.FilesFound++
			switch mark.FileTransferResult {
			case objects.TransferSuccess:
				r.Stats.FilesTransferred++
				r.Stats.BytesTransferred += mark.FileTransferBytes
			case objects.TransferFailed:
				r.Stats.FilesFailed++
			}
		case mark.IsCleanup:
			r.Stats.FilesCleanedUp += mark.CleanedUpObjectCount
		case mark.IsJob:
			if mark.JobSkipped {
				r.Stats.JobsSkipped++
			}
		}
	}

	//send statistics
	var gauge func(string, int64, float32) error
	if statter != nil {
		gauge = statter.Gauge
	} else {
		gauge = func(bucket string, value int64, rate float32) error { return nil }
	}
	gauge("last_run.jobs_skipped", r.Stats.JobsSkipped, 1.0)
	gauge("last_run.dirs_scanned", r.Stats.DirectoriesScanned, 1.0)
	gauge("last_run.files_found", r.Stats.FilesFound, 1.0)
	gauge("last_run.files_transfered", r.Stats.FilesTransferred, 1.0)
	gauge("last_run.files_failed", r.Stats.FilesFailed, 1.0)
	gauge("last_run.files_cleaned_up", r.Stats.FilesCleanedUp, 1.0)
	gauge("last_run.bytes_transfered", r.Stats.BytesTransferred, 1.0)
	if r.Stats.FilesFailed > 0 || r.Stats.DirectoriesFailed > 0 {
		gauge("last_run.success", 0, 1.0)
		r.ExitCode = 1
	} else {
		gauge("last_run.success", 1, 1.0)
		gauge("last_run.success_timestamp", time.Now().Unix(), 1.0)
		r.ExitCode = 0
	}

	//report results
	logg.Info("%d jobs skipped", r.Stats.JobsSkipped)
	logg.Info("%d dirs scanned, %d failed",
		r.Stats.DirectoriesScanned, r.Stats.DirectoriesFailed,
	)
	logg.Info("%d files found, %d transferred, %d failed",
		r.Stats.FilesFound, r.Stats.FilesTransferred, r.Stats.FilesFailed,
	)
	if r.Stats.FilesCleanedUp > 0 {
		logg.Info("%d old files cleaned up", r.Stats.FilesCleanedUp)
	}
	logg.Info("%d bytes transferred", r.Stats.BytesTransferred)

	duration := time.Since(r.StartTime)
	gauge("last_run.duration_seconds", int64(duration.Seconds()), 1.0)
	logg.Info("finished in %s", duration.String())
}
