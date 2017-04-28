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
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/context"

	"github.com/cactus/go-statsd-client/statsd"
	"github.com/ncw/swift"
)

func main() {
	startTime := time.Now()

	//read configuration
	config, errs := ReadConfiguration()
	if len(errs) > 0 {
		for _, err := range errs {
			Log(LogError, err.Error())
		}
		os.Exit(1)
	}

	// initialize statsd client
	var err error
	if config.Statsd.HostName != "" {
		statsd_client, err = statsd.NewClient(config.Statsd.HostName+":"+strconv.Itoa(config.Statsd.Port), config.Statsd.Prefix)
		// handle any errors
		if err != nil {
			Log(LogFatal, err.Error())
		}

		// make sure to clean up
		defer statsd_client.Close()
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
	err = conn.Authenticate()
	if err != nil {
		Log(LogFatal, err.Error())
	}
	PrepareTargets(&conn, config)
	PrepareJobs(&conn, config.Jobs)
	PrepareClients(config)

	//start workers
	Run(&SharedState{
		Configuration:   *config,
		Context:         context.Background(),
		SwiftConnection: &conn,
	})

	Gauge("last_run.duration_seconds", int64(time.Since(startTime).Seconds()), 1.0)
	Log(LogInfo, "finished in %s", time.Since(startTime).String())
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

//PrepareJobs fills those data structures in Job instances that require a
//swift.Connection.
func PrepareJobs(conn *swift.Connection, jobs []*Job) {
	for _, job := range jobs {
		//only need to fill IsFileTransferred if we want to abort transfers of
		//immutable files early
		if job.ImmutableFileRx == nil {
			continue
		}

		prefix := job.TargetPrefix
		if prefix != "" && !strings.HasSuffix(prefix, "/") {
			prefix += "/"
		}

		paths, err := conn.ObjectNames(job.TargetContainer, &swift.ObjectsOpts{
			Prefix: prefix,
		})
		if err != nil {
			Log(LogFatal,
				"could not list objects in Swift at %s/%s: %s",
				job.TargetContainer, prefix, err.Error(),
			)
		}
		job.IsFileTransferred = make(map[string]bool, len(paths))
		for _, path := range paths {
			job.IsFileTransferred[path] = true
		}
	}
}

//PrepareClients ensure http client SSL and or CA support setup
func PrepareClients(config *Configuration) {
	for _, job := range config.Jobs {
		tlsConfig := &tls.Config{}

		if job.ClientCertificatePath != "" {
			// Load client cert
			clientCertificate, err := tls.LoadX509KeyPair(job.ClientCertificatePath, job.ClientCertificateKeyPath)
			if err != nil {
				Log(LogFatal, "client certificate could not be loaded: %s", err.Error())
			}

			Log(LogDebug, "Client certificate %s loaded", job.ClientCertificatePath)

			// Setup HTTPS client
			tlsConfig.Certificates = []tls.Certificate{clientCertificate}
		}
		if job.ServerCAPath != "" {
			// Load server CA cert
			serverCA, err := ioutil.ReadFile(job.ServerCAPath)
			if err != nil {
				Log(LogFatal, "Server CA could not be loaded: %s", err.Error())
			}

			certPool := x509.NewCertPool()
			certPool.AppendCertsFromPEM(serverCA)

			Log(LogDebug, "Server CA %s loaded", job.ServerCAPath)

			// Setup HTTPS client
			tlsConfig.RootCAs = certPool
		}

		if job.ClientCertificatePath != "" || job.ServerCAPath != "" {
			tlsConfig.BuildNameToCertificate()
			// Overriding the transport for TLS, requires also Proxy to be set from ENV,
			// otherwise a set proxy will get lost
			transport := &http.Transport{TLSClientConfig: tlsConfig, Proxy: http.ProxyFromEnvironment}
			job.HTTPClient = &http.Client{Transport: transport}
		} else {
			job.HTTPClient = http.DefaultClient
		}
	}
}
