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
	"strconv"
	"time"

	"golang.org/x/net/context"

	"github.com/cactus/go-statsd-client/statsd"
	"github.com/sapcc/swift-http-import/pkg/util"
)

func main() {
	startTime := time.Now()

	//read configuration
	config, errs := ReadConfiguration()
	if len(errs) > 0 {
		for _, err := range errs {
			util.Log(util.LogError, err.Error())
		}
		os.Exit(1)
	}

	// initialize statsd client
	var err error
	if config.Statsd.HostName != "" {
		statsd_client, err = statsd.NewClient(config.Statsd.HostName+":"+strconv.Itoa(config.Statsd.Port), config.Statsd.Prefix)
		// handle any errors
		if err != nil {
			util.Log(util.LogFatal, err.Error())
		}

		// make sure to clean up
		defer statsd_client.Close()
	}

	//start workers
	exitCode := Run(&SharedState{
		Configuration: *config,
		Context:       context.Background(),
	})

	Gauge("last_run.duration_seconds", int64(time.Since(startTime).Seconds()), 1.0)
	util.Log(util.LogInfo, "finished in %s", time.Since(startTime).String())
	os.Exit(exitCode)
}
