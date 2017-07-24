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
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/ncw/swift"

	yaml "gopkg.in/yaml.v2"
)

//Configuration contains the contents of the configuration file.
type Configuration struct {
	Swift        SwiftCredentials `yaml:"swift"`
	WorkerCounts struct {
		Transfer uint
	} `yaml:"workers"`
	Statsd struct {
		HostName string `yaml:"hostname"`
		Port     int    `yaml:"port"`
		Prefix   string `yaml:"prefix"`
	} `yaml:"statsd"`
	JobConfigs []JobConfiguration `yaml:"jobs"`
	Jobs       []*Job             `yaml:"-"`
}

//ReadConfiguration reads the configuration file.
func ReadConfiguration() (*Configuration, []error) {
	if len(os.Args) != 2 {
		return nil, []error{fmt.Errorf("usage: %s <config-file>", os.Args[0])}
	}

	configBytes, err := ioutil.ReadFile(os.Args[1])
	if err != nil {
		return nil, []error{err}
	}

	var cfg Configuration
	err = yaml.Unmarshal(configBytes, &cfg)
	if err != nil {
		return nil, []error{err}
	}

	//set default values
	if cfg.WorkerCounts.Transfer == 0 {
		cfg.WorkerCounts.Transfer = 1
	}
	if cfg.Statsd.HostName != "" && cfg.Statsd.Port == 0 {
		cfg.Statsd.Port = 8125
	}
	if cfg.Statsd.Prefix == "" {
		cfg.Statsd.Prefix = "swift_http_import"
	}

	errors := cfg.Swift.Validate("swift")
	for idx, jobConfig := range cfg.JobConfigs {
		job, jobErrors := jobConfig.Compile(
			fmt.Sprintf("swift.jobs[%d]", idx),
			cfg.Swift,
		)
		cfg.Jobs = append(cfg.Jobs, job)
		errors = append(errors, jobErrors...)
	}

	return &cfg, errors
}

//JobConfiguration describes a transfer job in the configuration file.
type JobConfiguration struct {
	//basic options
	SourceRootURL            string `yaml:"from"`
	TargetContainerAndPrefix string `yaml:"to"`
	//behavior options
	ExcludePattern       string `yaml:"except"`
	IncludePattern       string `yaml:"only"`
	ImmutableFilePattern string `yaml:"immutable"`
	//auth options
	ClientCertificatePath    string `yaml:"cert"`
	ClientCertificateKeyPath string `yaml:"key"`
	ServerCAPath             string `yaml:"ca"`
}

//Job describes a transfer job at runtime.
type Job struct {
	Source            Location
	Target            *SwiftLocation
	HTTPClient        *http.Client
	ExcludeRx         *regexp.Regexp //pointers because nil signifies absence
	IncludeRx         *regexp.Regexp
	ImmutableFileRx   *regexp.Regexp
	IsFileTransferred map[string]bool //key = TargetPrefix + file path
}

//Compile validates the given JobConfiguration, then creates and prepares a Job from it.
func (cfg JobConfiguration) Compile(name string, creds SwiftCredentials) (job *Job, errors []error) {
	if cfg.SourceRootURL == "" {
		errors = append(errors, fmt.Errorf("missing value for %s.from", name))
	}
	if cfg.TargetContainerAndPrefix == "" {
		errors = append(errors, fmt.Errorf("missing value for %s.to", name))
	}
	// If one of the following is set, the other one needs also to be set
	if cfg.ClientCertificatePath != "" || cfg.ClientCertificateKeyPath != "" {
		if cfg.ClientCertificatePath == "" {
			errors = append(errors, fmt.Errorf("missing value for %s.cert", name))
		}
		if cfg.ClientCertificateKeyPath == "" {
			errors = append(errors, fmt.Errorf("missing value for %s.key", name))
		}
	}

	job = &Job{
		Source: URLLocation(cfg.SourceRootURL),
		Target: &SwiftLocation{
			Credentials:   creds,
			ContainerName: cfg.TargetContainerAndPrefix,
		},
	}

	//split "to" field into container and object name prefix if necessary
	if strings.Contains(cfg.TargetContainerAndPrefix, "/") {
		parts := strings.SplitN(cfg.TargetContainerAndPrefix, "/", 2)
		job.Target.ContainerName = parts[0]
		job.Target.ObjectPrefix = parts[1]
	}

	//compile patterns into regexes
	compileOptionalRegex := func(key, pattern string) *regexp.Regexp {
		if pattern == "" {
			return nil
		}
		rx, err := regexp.Compile(pattern)
		if err != nil {
			errors = append(errors, fmt.Errorf("malformed regex in %s.%s: %s", name, key, err.Error()))
		}
		return rx
	}
	job.ExcludeRx = compileOptionalRegex("except", cfg.ExcludePattern)
	job.IncludeRx = compileOptionalRegex("only", cfg.IncludePattern)
	job.ImmutableFileRx = compileOptionalRegex("immutable", cfg.ImmutableFilePattern)

	//ensure that connection to Swift exists and that target container is available
	err := job.Target.Connect()
	if err != nil {
		errors = append(errors, err)
	}

	errors = append(errors, job.prepareHTTPClient(cfg)...)

	err = job.prepareTransferredFilesLookup()
	if err != nil {
		errors = append(errors, err)
	}

	return
}

//Prepare HTTP client with SSL client certificate, if necessary.
func (job *Job) prepareHTTPClient(cfg JobConfiguration) (errors []error) {
	tlsConfig := &tls.Config{}

	if cfg.ClientCertificatePath != "" {
		// Load client cert
		clientCertificate, err := tls.LoadX509KeyPair(cfg.ClientCertificatePath, cfg.ClientCertificateKeyPath)
		if err != nil {
			errors = append(errors, fmt.Errorf("cannot load client certificate from %s: %s", cfg.ClientCertificatePath, err.Error()))
		}

		Log(LogDebug, "Client certificate %s loaded", cfg.ClientCertificatePath)
		tlsConfig.Certificates = []tls.Certificate{clientCertificate}
	}

	if cfg.ServerCAPath != "" {
		// Load server CA cert
		serverCA, err := ioutil.ReadFile(cfg.ServerCAPath)
		if err != nil {
			errors = append(errors, fmt.Errorf("cannot load CA certificate from %s: %s", cfg.ServerCAPath, err.Error()))
			Log(LogFatal, "Server CA could not be loaded: %s", err.Error())
		}

		certPool := x509.NewCertPool()
		certPool.AppendCertsFromPEM(serverCA)

		Log(LogDebug, "Server CA %s loaded", cfg.ServerCAPath)
		tlsConfig.RootCAs = certPool
	}

	if cfg.ClientCertificatePath != "" || cfg.ServerCAPath != "" {
		tlsConfig.BuildNameToCertificate()
		// Overriding the transport for TLS, requires also Proxy to be set from ENV,
		// otherwise a set proxy will get lost
		transport := &http.Transport{TLSClientConfig: tlsConfig, Proxy: http.ProxyFromEnvironment}
		job.HTTPClient = &http.Client{Transport: transport}
	} else {
		job.HTTPClient = http.DefaultClient
	}

	return
}

func (job *Job) prepareTransferredFilesLookup() error {
	//if we want to abort transfers of immutable files early...
	if job.ImmutableFileRx == nil {
		return nil
	}

	//...we need to first enumerate all files on the receiving side
	prefix := job.Target.ObjectPrefix
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	paths, err := job.Target.Connection.ObjectNames(job.Target.ContainerName, &swift.ObjectsOpts{
		Prefix: prefix,
	})
	if err != nil {
		return fmt.Errorf(
			"could not list objects in Swift at %s/%s: %s",
			job.Target.ContainerName, prefix, err.Error(),
		)
	}
	job.IsFileTransferred = make(map[string]bool, len(paths))
	for _, path := range paths {
		job.IsFileTransferred[path] = true
	}

	return nil
}
