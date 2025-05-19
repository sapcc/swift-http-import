// SPDX-FileCopyrightText: 2016-2017 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package actors

import (
	"context"

	"github.com/sapcc/go-bits/logg"

	"github.com/sapcc/swift-http-import/pkg/objects"
)

// Transferor is an actor that transfers files from a Source to a target SwiftLocation.
//
// Files to transfer are read from the `Input` channel until it is closed.
// For each input file, a report is sent into the `Report` channel.
type Transferor struct {
	Input  <-chan objects.File
	Output chan<- FileInfoForCleaner
	Report chan<- ReportEvent
}

// Run implements the Actor interface.
func (t *Transferor) Run(ctx context.Context) {
	done := ctx.Done()

	// main transfer loop - report successful and skipped transfers immediately,
	// but push back failed transfers for later retry
	aborted := false
	var filesToRetry []objects.File
LOOP:
	for {
		select {
		case <-done:
			aborted = true
			break LOOP
		case file, ok := <-t.Input:
			if !ok {
				break LOOP
			}
			result, size := file.PerformTransfer(ctx)
			if result == objects.TransferFailed {
				filesToRetry = append(filesToRetry, file)
			} else {
				t.Output <- FileInfoForCleaner{File: file, Failed: false}
				t.Report <- ReportEvent{IsFile: true, FileTransferResult: result, FileTransferBytes: size}
			}
		}
	}

	// retry transfer of failed files one more time
	if !aborted && len(filesToRetry) > 0 {
		logg.Info("retrying %d failed file transfers...", len(filesToRetry))
	}
	for _, file := range filesToRetry {
		result := objects.TransferFailed
		var size uint64
		// ...but only if we were not aborted (this is checked in every loop
		// iteration because the abort signal (i.e. Ctrl-C) could also happen
		// during this loop)
		if !aborted && ctx.Err() == nil {
			result, size = file.PerformTransfer(ctx)
		}
		t.Output <- FileInfoForCleaner{File: file, Failed: result == objects.TransferFailed}
		t.Report <- ReportEvent{IsFile: true, FileTransferResult: result, FileTransferBytes: size}
	}

	// if interrupt was received, consume all remaining input to get the Scraper
	// moving (it might be stuck trying to send into the File channel while the
	// channel's buffer is full)
	for range t.Input {
	}
}
