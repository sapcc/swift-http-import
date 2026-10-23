// SPDX-FileCopyrightText: 2016 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/sapcc/go-api-declarations/bininfo"
	"github.com/sapcc/go-bits/httpext"
	"github.com/sapcc/go-bits/logg"
	"github.com/sapcc/go-bits/osext"

	"github.com/sapcc/swift-http-import/pkg/actors"
	"github.com/sapcc/swift-http-import/pkg/objects"
)

func main() {
	startTime := time.Now()

	logg.ShowDebug = osext.GetenvBool("DEBUG")

	// setup a context that shuts down all pipeline actors when an interrupt signal is received
	ctx := httpext.ContextWithSIGINT(context.Background(), 1*time.Second)

	wrap := httpext.WrapTransport(&http.DefaultTransport)
	wrap.SetInsecureSkipVerify(osext.GetenvBool("INSECURE")) // for debugging with mitmproxy etc. (DO NOT SET IN PRODUCTION)
	wrap.SetOverrideUserAgent(bininfo.Component(), bininfo.VersionOr("dev"))

	// read arguments
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: swift-http-import <config-file>")
		fmt.Fprintln(os.Stderr, "   or: swift-http-import --version")
		os.Exit(1)
	}
	if os.Args[1] == "--version" {
		fmt.Println("swift-http-import " + bininfo.VersionOr("dev"))
		os.Exit(0)
	}

	// read configuration
	config, errs := objects.ReadConfiguration(ctx, os.Args[1])
	if len(errs) > 0 {
		for _, err := range errs {
			logg.Error(err.Error())
		}
		os.Exit(1)
	}

	// setup the Report actor
	reportChan := make(chan actors.ReportEvent)
	report := actors.Report{
		Input:     reportChan,
		Statsd:    config.Statsd,
		StartTime: startTime,
	}
	var wgReport sync.WaitGroup
	actors.Start(ctx, &report, &wgReport)

	// do the work
	runPipeline(ctx, config, reportChan)

	// shutdown Report actor
	close(reportChan)
	wgReport.Wait()
	os.Exit(report.ExitCode)
}

func runPipeline(ctx context.Context, config *objects.Configuration, report chan<- actors.ReportEvent) {
	// start the pipeline actors
	var wg sync.WaitGroup
	var wgTransfer sync.WaitGroup
	queue1 := make(chan objects.File, 10)              // will be closed by scraper when it's done
	queue2 := make(chan actors.FileInfoForCleaner, 10) // will be closed by us when all transferors are done

	actors.Start(ctx, &actors.Scraper{
		Jobs:   config.Jobs,
		Output: queue1,
		Report: report,
	}, &wg)

	for range config.WorkerCounts.Transfer {
		actors.Start(ctx, &actors.Transferor{
			Input:  queue1,
			Output: queue2,
			Report: report,
		}, &wg, &wgTransfer)
	}

	actors.Start(ctx, &actors.Cleaner{
		Input:  queue2,
		Report: report,
	}, &wg)

	// wait for transfer phase to finish
	wgTransfer.Wait()
	// signal to cleaner to start its work
	close(queue2)
	// wait for remaining workers to finish
	wg.Wait()
}
