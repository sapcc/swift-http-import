/*******************************************************************************
*
* Copyright 2017 SAP SE
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
	"io"
	"strings"

	"github.com/ncw/swift"
)

//Location describes a place from which files can be fetched.
type Location interface {
	//ListEntries implementations are in scraper.go
	ListEntries(job *Job, path string) (files []File, subdirectories []string)
	//GetFile implementations are in file.go
	GetFile(job *Job, path string, targetState FileState) (body io.ReadCloser, sourceState FileState, err error)
}

//FileState is used by Location.GetFile() to describe the state of a file.
type FileState struct {
	Etag         string
	LastModified string
	//the following fields are only used in `sourceState`, not `targetState`
	SkipTransfer bool
	ContentType  string
}

//URLLocation describes a location that's accessible by HTTP. Its value is its root URL.
type URLLocation string

//SwiftCredentials contains all parameters required to establish a Swift
//connection.
type SwiftCredentials struct {
	AuthURL           string `yaml:"auth_url"`
	UserName          string `yaml:"user_name"`
	UserDomainName    string `yaml:"user_domain_name"`
	ProjectName       string `yaml:"project_name"`
	ProjectDomainName string `yaml:"project_domain_name"`
	Password          string `yaml:"password"`
	RegionName        string `yaml:"region_name"`
}

func (c SwiftCredentials) cacheKey() string {
	return strings.Join([]string{
		c.AuthURL,
		c.UserName,
		c.UserDomainName,
		c.ProjectName,
		c.ProjectDomainName,
		c.Password,
		c.RegionName,
	}, "\000")
}

//Validate returns an empty list only if all required credentials are present.
func (c SwiftCredentials) Validate(name string) []error {
	var result []error

	if c.AuthURL == "" {
		result = append(result, fmt.Errorf("missing value for %s.auth_url", name))
	}
	if c.UserName == "" {
		result = append(result, fmt.Errorf("missing value for %s.user_name", name))
	}
	if c.UserDomainName == "" {
		result = append(result, fmt.Errorf("missing value for %s.user_domain_name", name))
	}
	if c.ProjectName == "" {
		result = append(result, fmt.Errorf("missing value for %s.project_name", name))
	}
	if c.ProjectDomainName == "" {
		result = append(result, fmt.Errorf("missing value for %s.project_domain_name", name))
	}
	if c.Password == "" {
		result = append(result, fmt.Errorf("missing value for %s.password", name))
	}

	return result
}

//SwiftLocation contains Swift credentials plus a location (container name and
//object prefix).
type SwiftLocation struct {
	Credentials   SwiftCredentials
	ContainerName string
	ObjectPrefix  string
	//Connection is filled by Connect().
	Connection *swift.Connection
}

var swiftConnectionCache = map[string]*swift.Connection{}

//Connect establishes the connection to Swift.
func (l *SwiftLocation) Connect() error {
	if l.Connection != nil {
		return nil
	}

	//create swift.Connection (but re-use if cached)
	key := l.Credentials.cacheKey()
	l.Connection = swiftConnectionCache[key]
	if l.Connection == nil {
		l.Connection = &swift.Connection{
			AuthVersion:  3,
			AuthUrl:      l.Credentials.AuthURL,
			UserName:     l.Credentials.UserName,
			Domain:       l.Credentials.UserDomainName,
			Tenant:       l.Credentials.ProjectName,
			TenantDomain: l.Credentials.ProjectDomainName,
			ApiKey:       l.Credentials.Password,
			Region:       l.Credentials.RegionName,
		}
		err := l.Connection.Authenticate()
		if err != nil {
			return fmt.Errorf("cannot authenticate to %s in %s@%s as %s@s: %s",
				l.Credentials.AuthURL,
				l.Credentials.ProjectName,
				l.Credentials.ProjectDomainName,
				l.Credentials.UserName,
				l.Credentials.UserDomainName,
				err.Error(),
			)
		}
		swiftConnectionCache[key] = l.Connection
	}

	//create target container if missing
	err := l.Connection.ContainerCreate(l.ContainerName, nil)
	if err != nil {
		return fmt.Errorf("cannot create container %s in %s@%s as %s@s: %s",
			l.ContainerName,
			l.Credentials.ProjectName,
			l.Credentials.ProjectDomainName,
			l.Credentials.UserName,
			l.Credentials.UserDomainName,
			err.Error(),
		)
	}
	return nil
}
