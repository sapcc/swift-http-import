// SPDX-FileCopyrightText: 2018 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package actors

import (
	"context"
	"sort"

	"github.com/majewsky/schwift/v2"
	"github.com/sapcc/go-bits/errext"
	"github.com/sapcc/go-bits/logg"

	"github.com/sapcc/swift-http-import/pkg/objects"
	"github.com/sapcc/swift-http-import/pkg/util"
)

// FileInfoForCleaner contains information about a transferred file for the Cleaner actor.
type FileInfoForCleaner struct {
	objects.File
	Failed bool
}

// Cleaner is an actor that cleans up unknown objects on the target side (i.e.
// those objects which do not exist on the source side).
type Cleaner struct {
	Input  <-chan FileInfoForCleaner
	Report chan<- ReportEvent
}

// Run implements the Actor interface.
func (c *Cleaner) Run(ctx context.Context) {
	isJobFailed := make(map[*objects.Job]bool)
	isFileTransferred := make(map[*objects.Job]map[string]bool) // string = object name incl. prefix (if any)

	// collect information about transferred files from the transferors
	// (we don't need to check Context.Done in the loop; when the process is
	// interrupted, main() will close our Input and we will move on)
	for info := range c.Input {
		// ignore all files in jobs where no cleanup is configured
		job := info.Job
		if job.Cleanup.Strategy == objects.KeepUnknownFiles {
			continue
		}

		if info.Failed {
			isJobFailed[job] = true
		}

		m, exists := isFileTransferred[job]
		if !exists {
			m = make(map[string]bool)
			isFileTransferred[job] = m
		}
		m[info.File.TargetObject().Name()] = true
	}
	if ctx.Err() != nil {
		logg.Info("skipping cleanup phase: interrupt was received")
		return
	}

	// collect information about incomplete scrapes (this is not safe to do in the
	// above loop because the scraper job might be writing the
	// Job.IsScrapingIncomplete attribute concurrently; at this point the scraper
	// is definitely done, so these attributes are safe to read without risking a
	// data race)
	for job := range isFileTransferred {
		if job.IsScrapingIncomplete {
			isJobFailed[job] = true
		}
	}
	if len(isJobFailed) > 0 {
		logg.Info(
			"skipping cleanup phase for %d job(s) because of failed file transfers",
			len(isJobFailed))
	}

	// perform cleanup if it is safe to do so
	for job, transferred := range isFileTransferred {
		if ctx.Err() != nil {
			// interrupt received
			return
		}
		if !isJobFailed[job] {
			c.performCleanup(ctx, job, transferred)
		}
	}
}

func (c *Cleaner) performCleanup(ctx context.Context, job *objects.Job, isFileTransferred map[string]bool) {
	// collect objects to cleanup
	var objs []*schwift.Object
	for objectName := range job.Target.FileExists {
		if isFileTransferred[objectName] {
			continue
		}
		objs = append(objs, job.Target.Container.Object(objectName))
	}
	sort.Slice(objs, func(i, j int) bool {
		return objs[i].Name() < objs[j].Name()
	})

	if job.Cleanup.Strategy != objects.KeepUnknownFiles {
		logg.Info("starting cleanup of %d objects on target side", len(objs))
	}

	// perform cleanup according to selected strategy
	switch job.Cleanup.Strategy {
	case objects.ReportUnknownFiles:
		for _, obj := range objs {
			logg.Info("found unknown object on target side: %s", obj.FullName())
		}

	case objects.DeleteUnknownFiles:
		numDeleted, _, err := job.Target.Container.Account().BulkDelete(ctx, objs, nil, nil)
		if numDeleted < 0 {
			numDeleted = 0
		}
		c.Report <- ReportEvent{IsCleanup: true, CleanedUpObjectCount: util.AtLeastZero(numDeleted)}
		if err != nil {
			logg.Error("cleanup of %d objects on target side failed: %s", (len(objs) - numDeleted), err.Error())
			if berr, ok := errext.As[schwift.BulkError](err); ok {
				for _, oerr := range berr.ObjectErrors {
					logg.Error("DELETE " + oerr.Error())
				}
			}
		}
	}
}
