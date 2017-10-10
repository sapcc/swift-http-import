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
	"fmt"
	"io/ioutil"
	"os"
	"regexp"

	"github.com/sapcc/swift-http-import/pkg/objects"

	yaml "gopkg.in/yaml.v2"
)

//Configuration contains the contents of the configuration file.
type Configuration struct {
	Swift        objects.SwiftLocation `yaml:"swift"`
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

	cfg.Swift.ValidateIgnoreEmptyContainer = true
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
	Source SourceUnmarshaler      `yaml:"from"`
	Target *objects.SwiftLocation `yaml:"to"`
	//behavior options
	ExcludePattern       string `yaml:"except"`
	IncludePattern       string `yaml:"only"`
	ImmutableFilePattern string `yaml:"immutable"`
}

//SourceUnmarshaler provides a yaml.Unmarshaler implementation for the Source interface.
type SourceUnmarshaler struct {
	src objects.Source
}

//UnmarshalYAML implements the yaml.Unmarshaler interface.
func (u *SourceUnmarshaler) UnmarshalYAML(unmarshal func(interface{}) error) error {
	//unmarshal as map
	var data map[string]interface{}
	err := unmarshal(&data)
	if err != nil {
		return err
	}

	//look at keys to determine whether this is a URLSource or a SwiftSource
	if _, exists := data["url"]; exists {
		u.src = &objects.URLSource{}
	} else {
		u.src = &objects.SwiftLocation{}
	}
	return unmarshal(u.src)
}

//Job describes a transfer job at runtime.
type Job struct {
	Source  objects.Source
	Target  *objects.SwiftLocation
	Matcher objects.Matcher
}

//Compile validates the given JobConfiguration, then creates and prepares a Job from it.
func (cfg JobConfiguration) Compile(name string, swift objects.SwiftLocation) (job *Job, errors []error) {
	if cfg.Source.src == nil {
		errors = append(errors, fmt.Errorf("missing value for %s.from", name))
	} else {
		errors = append(errors, cfg.Source.src.Validate(name+".from")...)
	}
	if cfg.Target == nil {
		errors = append(errors, fmt.Errorf("missing value for %s.to", name))
	} else {
		//target inherits connection parameters from global Swift credentials
		cfg.Target.AuthURL = swift.AuthURL
		cfg.Target.UserName = swift.UserName
		cfg.Target.UserDomainName = swift.UserDomainName
		cfg.Target.ProjectName = swift.ProjectName
		cfg.Target.ProjectDomainName = swift.ProjectDomainName
		cfg.Target.Password = swift.Password
		cfg.Target.RegionName = swift.RegionName
		errors = append(errors, cfg.Target.Validate(name+".to")...)
	}

	job = &Job{
		Source: cfg.Source.src,
		Target: cfg.Target,
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
	job.Matcher.ExcludeRx = compileOptionalRegex("except", cfg.ExcludePattern)
	job.Matcher.IncludeRx = compileOptionalRegex("only", cfg.IncludePattern)
	job.Matcher.ImmutableFileRx = compileOptionalRegex("immutable", cfg.ImmutableFilePattern)

	//do not try connecting to Swift if credentials are invalid etc.
	if len(errors) > 0 {
		return
	}

	//ensure that connection to Swift exists and that target container is available
	err := job.Source.Connect()
	if err != nil {
		errors = append(errors, err)
	}
	err = job.Target.Connect()
	if err != nil {
		errors = append(errors, err)
	}

	err = job.Target.DiscoverExistingFiles(job.Matcher)
	if err != nil {
		errors = append(errors, err)
	}

	return
}
