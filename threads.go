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
	"sync"

	"github.com/sapcc/swift-http-import/pkg/actors"
	"github.com/sapcc/swift-http-import/pkg/objects"

	"golang.org/x/net/context"
)

//SharedState contains all the stuff shared between all worker threads.
type SharedState struct {
	objects.Configuration
	Context   context.Context
	WaitGroup sync.WaitGroup
	Report    chan<- actors.ReportEvent
}

//Run starts and orchestrates the various worker threads.
func Run(state *SharedState) {
	//setup a simple linear pipeline of workers (it should be fairly trivial to
	//scale this out to multiple workers later)
	queue := make(chan objects.File, 10)
	s := actors.Scraper{
		Context: state.Context,
		Jobs:    state.Configuration.Jobs,
		Output:  queue,
		Report:  state.Report,
	}
	go s.Run(&state.WaitGroup)

	for i := uint(0); i < state.WorkerCounts.Transfer; i++ {
		t := actors.Transferor{
			Context: state.Context,
			Input:   queue,
			Report:  state.Report,
		}
		go t.Run(&state.WaitGroup)
	}

	//wait for all of them to return
	state.WaitGroup.Wait()
}
