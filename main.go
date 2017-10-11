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
	"time"

	"golang.org/x/net/context"

	"github.com/sapcc/swift-http-import/pkg/actors"
	"github.com/sapcc/swift-http-import/pkg/objects"
	"github.com/sapcc/swift-http-import/pkg/util"
)

func main() {
	startTime := time.Now()

	//read configuration
	config, errs := objects.ReadConfiguration()
	if len(errs) > 0 {
		for _, err := range errs {
			util.Log(util.LogError, err.Error())
		}
		os.Exit(1)
	}

	//receive SIGINT/SIGTERM signals
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)

	//setup a context that cancels the workers when one of the signals above is received
	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()
	go func() {
		<-sigs
		cancelFunc()
	}()

	//setup the Report actor
	reportChan := make(chan actors.ReportEvent)
	report := actors.Report{
		Input:     reportChan,
		Done:      ctx.Done(),
		Statsd:    config.Statsd,
		StartTime: startTime,
	}
	var wg sync.WaitGroup
	go report.Run(&wg)

	//start workers
	Run(&SharedState{
		Configuration: *config,
		Context:       ctx,
		Report:        reportChan,
	})

	//shutdown Report actor
	close(reportChan)
	wg.Wait()

	os.Exit(report.ExitCode)
}
