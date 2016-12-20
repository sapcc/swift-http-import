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
	"sync"

	"golang.org/x/net/context"

	"github.com/ncw/swift"
)

func main() {
	//read configuration
	config, errs := ReadConfiguration()
	if len(errs) > 0 {
		for _, err := range errs {
			Log(LogError, err.Error())
		}
		os.Exit(1)
	}

	//initialize Swift connection
	conn := swift.Connection{
		AuthVersion:  3,
		AuthUrl:      config.Swift.AuthURL,
		UserName:     config.Swift.UserName,
		Domain:       config.Swift.UserDomainName,
		Tenant:       config.Swift.ProjectName,
		TenantDomain: config.Swift.ProjectDomainName,
		ApiKey:       config.Swift.Password,
		Region:       config.Swift.RegionName,
	}
	err := conn.Authenticate()
	if err != nil {
		Log(LogFatal, err.Error())
	}
	PrepareTargets(&conn, config)

	//start workers
	Run(&SharedState{
		Configuration:   *config,
		Context:         context.Background(),
		SwiftConnection: &conn,
	})
}

//PrepareTargets ensures that all the target containers exist.
func PrepareTargets(conn *swift.Connection, config *Configuration) {
	//de-duplicate the target container names
	targetContainers := make(map[string]struct{})
	for _, job := range config.Jobs {
		targetContainers[job.TargetContainer] = struct{}{}
	}

	//create all containers simultaneously
	var wg sync.WaitGroup
	wg.Add(len(targetContainers))

	for containerName := range targetContainers {
		containerName := containerName //shadow mutable loop variable
		go func() {
			err := conn.ContainerCreate(containerName, nil)
			if err != nil {
				Log(LogFatal, "could not create target container %s: %s", containerName, err.Error())
			}
			wg.Done()
		}()
	}

	wg.Wait()
}
