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
	"context"
	"math"
	"strconv"
	"time"

	"github.com/cactus/go-statsd-client/v5/statsd"
	"github.com/sapcc/go-bits/logg"

	"github.com/sapcc/swift-http-import/pkg/objects"
)

// ReportEvent counts either a directory that was scraped, or a file that was
// found (and maybe transferred). It is consumed by the Report actor.
type ReportEvent struct {
	IsJob      bool
	JobSkipped bool

	IsDirectory     bool
	DirectoryFailed bool

	IsFile             bool
	FileTransferResult objects.TransferResult
	FileTransferBytes  uint64

	IsCleanup            bool
	CleanedUpObjectCount uint64
}

// Report is an actor that counts scraped directories and transferred files.
// It emits StatsD metrics (if desired), logs the final report, and decides
// whether to exit with an error status.
//
// Events are read from the `Input` channel until it is closed.
// The `Done` channel can be closed to interrupt the actor.
// If the `Statter` is not nil, statsd metrics will be emitted.
// The `StartTime` is used to measure this run's duration at the end.
// The `ExitCode` can be read after the actor is done.
type Report struct {
	Input     <-chan ReportEvent
	Statsd    objects.StatsdConfiguration
	StartTime time.Time
	ExitCode  int
	stats     Stats
}

// Stats contains the report statistics
type Stats struct {
	DirectoriesScanned uint64
	DirectoriesFailed  uint64
	FilesFound         uint64
	FilesFailed        uint64
	FilesTransferred   uint64
	FilesCleanedUp     uint64
	BytesTransferred   uint64
	JobsSkipped        uint64
	Duration           time.Duration
}

// Stats returns a copy of stats member.
func (r *Report) Stats() Stats {
	return r.stats
}

// Run implements the Actor interface.
func (r *Report) Run(ctx context.Context) {
	var statter statsd.Statter

	// initialize statsd client
	if r.Statsd.HostName != "" {
		var err error
		statter, err = statsd.NewClientWithConfig(&statsd.ClientConfig{
			Address: r.Statsd.HostName + ":" + strconv.Itoa(r.Statsd.Port),
			Prefix:  r.Statsd.Prefix,
		})
		// handle any errors
		if err != nil {
			logg.Fatal(err.Error())
		}

		// make sure to clean up
		defer statter.Close()
	}

	// collect tally marks until done or aborted
	for mark := range r.Input {
		switch {
		case mark.IsDirectory:
			r.stats.DirectoriesScanned++
			if mark.DirectoryFailed {
				r.stats.DirectoriesFailed++
			}
		case mark.IsFile:
			r.stats.FilesFound++
			switch mark.FileTransferResult {
			case objects.TransferSuccess:
				r.stats.FilesTransferred++
				r.stats.BytesTransferred += mark.FileTransferBytes
			case objects.TransferFailed:
				r.stats.FilesFailed++
			}
		case mark.IsCleanup:
			r.stats.FilesCleanedUp += mark.CleanedUpObjectCount
		case mark.IsJob:
			if mark.JobSkipped {
				r.stats.JobsSkipped++
			}
		}
	}

	// send statistics
	var (
		gaugeI64 = func(string, int64) {}
		gaugeU64 = func(string, uint64) {}
	)
	if statter != nil {
		gaugeI64 = func(bucket string, value int64) {
			err := statter.Gauge(bucket, value, 1.0) // 1.0 is the `rate` argument
			if err != nil {
				logg.Error("statsd: could not submit value %d for bucket %q: %s", value, bucket, err.Error())
			}
		}
		gaugeU64 = func(bucket string, value uint64) {
			if value > math.MaxInt64 {
				logg.Error("statsd: value %d for bucket %q is out of range for int64", value, bucket)
			}
			gaugeI64(bucket, int64(value)) //nolint:gosec // the linter is stupid, it complains about the integer conversion that we literally just checked
		}
	}

	gaugeU64("last_run.jobs_skipped", r.stats.JobsSkipped)
	gaugeU64("last_run.dirs_scanned", r.stats.DirectoriesScanned)
	gaugeU64("last_run.files_found", r.stats.FilesFound)
	gaugeU64("last_run.files_transfered", r.stats.FilesTransferred)
	gaugeU64("last_run.files_failed", r.stats.FilesFailed)
	gaugeU64("last_run.files_cleaned_up", r.stats.FilesCleanedUp)
	gaugeU64("last_run.bytes_transfered", r.stats.BytesTransferred)
	if r.stats.FilesFailed > 0 || r.stats.DirectoriesFailed > 0 {
		gaugeU64("last_run.success", 0)
		r.ExitCode = 1
	} else {
		gaugeU64("last_run.success", 1)
		gaugeI64("last_run.success_timestamp", time.Now().Unix())
		r.ExitCode = 0
	}

	// report results
	logg.Info("%d jobs skipped", r.stats.JobsSkipped)
	logg.Info("%d dirs scanned, %d failed",
		r.stats.DirectoriesScanned, r.stats.DirectoriesFailed,
	)
	logg.Info("%d files found, %d transferred, %d failed",
		r.stats.FilesFound, r.stats.FilesTransferred, r.stats.FilesFailed,
	)
	if r.stats.FilesCleanedUp > 0 {
		logg.Info("%d old files cleaned up", r.stats.FilesCleanedUp)
	}
	logg.Info("%d bytes transferred", r.stats.BytesTransferred)

	r.stats.Duration = time.Since(r.StartTime)
	gaugeI64("last_run.duration_seconds", int64(r.stats.Duration.Seconds()))
	logg.Info("finished in %s", r.stats.Duration.String())
}
