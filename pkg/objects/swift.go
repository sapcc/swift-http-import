/*******************************************************************************
*
* Copyright 2016-2017 SAP SE
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

package objects

import (
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ncw/swift"
	"github.com/sapcc/swift-http-import/pkg/util"
)

//SwiftLocation contains all parameters required to establish a Swift connection.
//It implements the Source interface, but is also used on the target side.
type SwiftLocation struct {
	AuthURL           string `yaml:"auth_url"`
	UserName          string `yaml:"user_name"`
	UserDomainName    string `yaml:"user_domain_name"`
	ProjectName       string `yaml:"project_name"`
	ProjectDomainName string `yaml:"project_domain_name"`
	Password          string `yaml:"password"`
	RegionName        string `yaml:"region_name"`
	ContainerName     string `yaml:"container"`
	ObjectNamePrefix  string `yaml:"object_prefix"`
	//configuration for Validate()
	ValidateIgnoreEmptyContainer bool `yaml:"-"`
	//Connection is filled by Connect().
	Connection *swift.Connection `yaml:"-"`
	//FileExists is filled by DiscoverExistingFiles(). The keys are object names
	//including the ObjectNamePrefix, if any.
	FileExists map[string]bool `yaml:"-"`
}

func (s SwiftLocation) cacheKey() string {
	return strings.Join([]string{
		s.AuthURL,
		s.UserName,
		s.UserDomainName,
		s.ProjectName,
		s.ProjectDomainName,
		s.Password,
		s.RegionName,
	}, "\000")
}

//Validate returns an empty list only if all required credentials are present.
func (s SwiftLocation) Validate(name string) []error {
	var result []error

	if s.AuthURL == "" {
		result = append(result, fmt.Errorf("missing value for %s.auth_url", name))
	}
	if s.UserName == "" {
		result = append(result, fmt.Errorf("missing value for %s.user_name", name))
	}
	if s.UserDomainName == "" {
		result = append(result, fmt.Errorf("missing value for %s.user_domain_name", name))
	}
	if s.ProjectName == "" {
		result = append(result, fmt.Errorf("missing value for %s.project_name", name))
	}
	if s.ProjectDomainName == "" {
		result = append(result, fmt.Errorf("missing value for %s.project_domain_name", name))
	}
	if s.Password == "" {
		result = append(result, fmt.Errorf("missing value for %s.password", name))
	}
	if !s.ValidateIgnoreEmptyContainer && s.ContainerName == "" {
		result = append(result, fmt.Errorf("missing value for %s.container", name))
	}

	return result
}

var swiftConnectionCache = map[string]*swift.Connection{}

//Connect implements the Source interface. It establishes the connection to Swift.
func (s *SwiftLocation) Connect() error {
	if s.Connection != nil {
		return nil
	}

	//create swift.Connection (but re-use if cached)
	key := s.cacheKey()
	s.Connection = swiftConnectionCache[key]
	if s.Connection == nil {
		s.Connection = &swift.Connection{
			AuthVersion:  3,
			AuthUrl:      s.AuthURL,
			UserName:     s.UserName,
			Domain:       s.UserDomainName,
			Tenant:       s.ProjectName,
			TenantDomain: s.ProjectDomainName,
			ApiKey:       s.Password,
			Region:       s.RegionName,
		}
		err := s.Connection.Authenticate()
		if err != nil {
			return fmt.Errorf("cannot authenticate to %s in %s@%s as %s@%s: %s",
				s.AuthURL,
				s.ProjectName,
				s.ProjectDomainName,
				s.UserName,
				s.UserDomainName,
				err.Error(),
			)
		}
		swiftConnectionCache[key] = s.Connection
	}

	//create target container if missing
	return s.EnsureContainerExists(s.ContainerName)
}

//EnsureContainerExists creates the given container in this Swift account, if
//it does not exist yet.
func (s *SwiftLocation) EnsureContainerExists(containerName string) error {
	err := s.Connection.ContainerCreate(containerName, nil)
	if err != nil {
		return fmt.Errorf("cannot create container %s in %s@%s as %s@%s: %s",
			containerName,
			s.ProjectName,
			s.ProjectDomainName,
			s.UserName,
			s.UserDomainName,
			err.Error(),
		)
	}
	return nil
}

//ListEntries implements the Source interface.
func (s *SwiftLocation) ListEntries(path string) ([]string, *ListEntriesError) {
	objectPath := filepath.Join(s.ObjectNamePrefix, strings.TrimPrefix(path, "/"))
	if objectPath != "" && !strings.HasSuffix(objectPath, "/") {
		objectPath += "/"
	}
	util.Log(util.LogDebug, "listing objects at %s/%s", s.ContainerName, objectPath)

	names, err := s.Connection.ObjectNamesAll(s.ContainerName, &swift.ObjectsOpts{
		Prefix:    objectPath,
		Delimiter: '/',
	})
	if err != nil {
		return nil, &ListEntriesError{
			Location: s.ContainerName + "/" + "objectPath",
			Message:  "GET failed: " + err.Error(),
		}
	}

	//ObjectNamesAll returns full names, but we want only the basenames
	for idx, name := range names {
		isDir := strings.HasSuffix(name, "/")
		names[idx] = filepath.Base(name)
		if isDir {
			names[idx] += "/"
		}
	}
	return names, nil
}

//GetFile implements the Source interface.
func (s *SwiftLocation) GetFile(path string, targetState FileState) (io.ReadCloser, FileState, error) {
	objectPath := filepath.Join(s.ObjectNamePrefix, path)

	reqHeaders := make(swift.Headers)
	if targetState.Etag != "" {
		reqHeaders["If-None-Match"] = targetState.Etag
	}
	if targetState.LastModified != "" {
		reqHeaders["If-Modified-Since"] = targetState.LastModified
	}

	body, respHeaders, err := s.Connection.ObjectOpen(s.ContainerName, objectPath, false, reqHeaders)

	switch err {
	case nil:
		sizeBytes, err := body.Length() //this just reads Content-Length despite not looking like it
		if err != nil {
			util.Log(util.LogError, "invalid Content-Length header for object %s/%s", s.ContainerName, objectPath)
			sizeBytes = -1
		}
		var expiryTime *time.Time
		if expiryStr := respHeaders["X-Delete-At"]; expiryStr != "" {
			expiryUnix, err := strconv.ParseInt(expiryStr, 10, 64)
			if err == nil {
				t := time.Unix(expiryUnix, 0)
				expiryTime = &t
			} else {
				util.Log(util.LogError, "invalid X-Delete-At header for object %s/%s", s.ContainerName, objectPath)
			}
		}

		return body, FileState{
			Etag:         respHeaders["Etag"],
			LastModified: respHeaders["Last-Modified"],
			SizeBytes:    sizeBytes,
			ExpiryTime:   expiryTime,
			ContentType:  respHeaders["Content-Type"],
		}, nil
	case swift.NotModified:
		return nil, FileState{SkipTransfer: true}, nil
	default:
		return nil, FileState{}, err
	}
}

//DiscoverExistingFiles finds all objects that currently exist in this location
//(i.e. in this Swift container below the given object name prefix) and fills
//s.FileExists accordingly.
//
//The given Matcher is used to find out which files are to be considered as
//belonging to the transfer job in question.
func (s *SwiftLocation) DiscoverExistingFiles(matcher Matcher) error {

	prefix := s.ObjectNamePrefix
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	paths, err := s.Connection.ObjectNames(s.ContainerName, &swift.ObjectsOpts{
		Prefix: prefix,
	})
	if err != nil {
		return fmt.Errorf(
			"could not list objects in Swift at %s/%s: %s",
			s.ContainerName, prefix, err.Error(),
		)
	}

	s.FileExists = make(map[string]bool, len(paths))
	for _, path := range paths {
		pathForMatching := strings.TrimPrefix(path, prefix)
		if matcher.CheckRecursive(pathForMatching) == "" {
			s.FileExists[path] = true
		}
	}

	return nil
}
