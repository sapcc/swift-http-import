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
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/sapcc/go-bits/logg"
	"github.com/sapcc/swift-http-import/pkg/actors"
	"github.com/sapcc/swift-http-import/pkg/objects"
	"github.com/sapcc/swift-http-import/pkg/util"
)

func main() {
	startTime := time.Now()

	//read arguments
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: swift-http-import <config-file>")
		fmt.Fprintln(os.Stderr, "   or: swift-http-import --version")
		os.Exit(1)
	}
	if os.Args[1] == "--version" {
		fmt.Println("swift-http-import " + util.Version)
		os.Exit(0)
	}

	//read configuration
	config, errs := objects.ReadConfiguration(os.Args[1])
	if len(errs) > 0 {
		for _, err := range errs {
			logg.Error(err.Error())
		}
		os.Exit(1)
	}

	//setup the Report actor
	reportChan := make(chan actors.ReportEvent)
	report := actors.Report{
		Input:     reportChan,
		Statsd:    config.Statsd,
		StartTime: startTime,
	}
	var wgReport sync.WaitGroup
	actors.Start(&report, &wgReport)

	//do the work
	runPipeline(config, reportChan)

	//shutdown Report actor
	close(reportChan)
	wgReport.Wait()
	os.Exit(report.ExitCode)
}

func runPipeline(config *objects.Configuration, report chan<- actors.ReportEvent) {
	//receive SIGINT/SIGTERM signals
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)

	//setup a context that shuts down all pipeline actors when one of the signals above is received
	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()
	go func() {
		<-sigs
		logg.Error("Interrupt received! Shutting down...")
		cancelFunc()
	}()

	//start the pipeline actors
	var wg sync.WaitGroup
	var wgTransfer sync.WaitGroup
	queue1 := make(chan objects.File, 10)              //will be closed by scraper when it's done
	queue2 := make(chan actors.FileInfoForCleaner, 10) //will be closed by us when all transferors are done

	actors.Start(&actors.Scraper{
		Context: ctx,
		Jobs:    config.Jobs,
		Output:  queue1,
		Report:  report,
	}, &wg)

	for i := uint(0); i < config.WorkerCounts.Transfer; i++ {
		actors.Start(&actors.Transferor{
			Context: ctx,
			Input:   queue1,
			Output:  queue2,
			Report:  report,
		}, &wg, &wgTransfer)
	}

	actors.Start(&actors.Cleaner{
		Context: ctx,
		Input:   queue2,
		Report:  report,
	}, &wg)

	//wait for transfer phase to finish
	wgTransfer.Wait()
	//signal to cleaner to start its work
	close(queue2)
	//wait for remaining workers to finish
	wg.Wait()

	// signal.Reset(os.Interrupt, syscall.SIGTERM)
}
