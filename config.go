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
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	yaml "gopkg.in/yaml.v2"
)

//Job describes a single mirroring job.
type Job struct {
	SourceRootURL            string `yaml:"from"`
	TargetContainer          string `yaml:"to"`
	TargetPrefix             string `yaml:"-"`
	ClientCertificatePath    string `yaml:"cert"`
	ClientCertificateKeyPath string `yaml:"key"`
	ServerCAPath             string `yaml:"ca"`
	HTTPClient               *http.Client
	//undocumented "features" for quirky sources
	SkipLeadingPathElementsCount uint `yaml:"skip_leading_path_elements_count"`
}

//Configuration contains the contents of the configuration file.
type Configuration struct {
	Swift struct {
		AuthURL           string `yaml:"auth_url"`
		UserName          string `yaml:"user_name"`
		UserDomainName    string `yaml:"user_domain_name"`
		ProjectName       string `yaml:"project_name"`
		ProjectDomainName string `yaml:"project_domain_name"`
		Password          string `yaml:"password"`
		RegionName        string `yaml:"region_name"`
	}
	Jobs []*Job
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

	for _, job := range cfg.Jobs {
		//split "to" field into container and object name prefix if necessary
		if strings.Contains(job.TargetContainer, "/") {
			parts := strings.SplitN(job.TargetContainer, "/", 2)
			job.TargetContainer = parts[0]
			job.TargetPrefix = parts[1]
		}
	}

	return &cfg, cfg.Validate()
}

//Validate returns an empty list only if the configuration is valid.
func (cfg Configuration) Validate() []error {
	var result []error

	if cfg.Swift.AuthURL == "" {
		result = append(result, errors.New("missing value for swift.auth_url"))
	}
	if cfg.Swift.UserName == "" {
		result = append(result, errors.New("missing value for swift.user_name"))
	}
	if cfg.Swift.UserDomainName == "" {
		result = append(result, errors.New("missing value for swift.user_domain_name"))
	}
	if cfg.Swift.ProjectName == "" {
		result = append(result, errors.New("missing value for swift.project_name"))
	}
	if cfg.Swift.ProjectDomainName == "" {
		result = append(result, errors.New("missing value for swift.project_domain_name"))
	}
	if cfg.Swift.Password == "" {
		result = append(result, errors.New("missing value for swift.password"))
	}
	if cfg.Swift.RegionName == "" {
		result = append(result, errors.New("missing value for swift.region_name"))
	}

	for idx, job := range cfg.Jobs {
		if job.SourceRootURL == "" {
			result = append(result, fmt.Errorf("missing value for swift.jobs[%d].from", idx))
		}
		if job.TargetContainer == "" {
			result = append(result, fmt.Errorf("missing value for swift.jobs[%d].to", idx))
		}
		// If one of the following is set, the other one needs also to be set
		if job.ClientCertificatePath != "" || job.ClientCertificateKeyPath != "" {
			if job.ClientCertificatePath == "" {
				result = append(result, fmt.Errorf("missing value for swift.jobs[%d].cert", idx))
			}
			if job.ClientCertificateKeyPath == "" {
				result = append(result, fmt.Errorf("missing value for swift.jobs[%d].key", idx))
			}
		}
	}

	return result
}
