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
	"log"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/ncw/swift"
)

//File describes a single file which is mirrored as part of a Job.
type File struct {
	Job  *Job
	Path string
}

//SourceURL returns the URL of this file at its source.
func (f File) SourceURL() string {
	return URLPathJoin(f.Job.SourceRootURL, f.Path)
}

//TargetObjectName returns the object name of this file in the target container.
func (f File) TargetObjectName() string {
	objectName := strings.TrimPrefix(f.Path, "/")
	if f.Job.TargetPrefix == "" {
		return objectName
	}
	return filepath.Join(f.Job.TargetPrefix, objectName)
}

//TransferResult is the return type for PerformTransfer().
type TransferResult uint

const (
	//TransferSuccess means that the file was newer on the source and was sent
	//to the target.
	TransferSuccess TransferResult = iota
	//TransferSkipped means that the file was the same on both sides and
	//nothing was transferred.
	TransferSkipped
	//TransferFailed means that an error occurred and was logged.
	TransferFailed
)

//PerformTransfer transfers this file from the source to the target.
//The return value indicates if the transfer finished successfully.
func (f File) PerformTransfer(conn *swift.Connection) TransferResult {
	Log(LogDebug, "transferring %s", f.SourceURL())

	//query the file metadata at the target
	_, headers, err := conn.Object(
		f.Job.TargetContainer,
		f.TargetObjectName(),
	)
	if err != nil {
		if err == swift.ObjectNotFound {
			headers = swift.Headers{}
		} else {
			//log all other errors and skip the file (we don't want to waste
			//bandwidth downloading stuff if there is reasonable doubt that we will
			//not be able to upload it to Swift)
			log.Printf("skipping %s: HEAD %s/%s failed: %s",
				f.SourceURL(),
				f.Job.TargetContainer, f.TargetObjectName(),
				err.Error(),
			)
			return TransferFailed
		}
	}

	//prepare request to retrieve from source, taking advantage of Etag and
	//Last-Modified where possible
	req, err := http.NewRequest("GET", f.SourceURL(), nil)
	if err != nil {
		log.Printf("skipping %s: GET failed: %s", f.SourceURL(), err.Error())
		return TransferFailed
	}
	metadata := headers.ObjectMetadata()
	if etag := metadata["source-etag"]; etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	if mtime := metadata["source-last-modified"]; mtime != "" {
		req.Header.Set("If-Modified-Since", mtime)
	}

	//retrieve file from source
	response, err := f.Job.HTTPClient.Do(req)
	if err != nil {
		log.Printf("skipping %s: GET failed: %s", f.SourceURL(), err.Error())
		return TransferFailed
	}
	defer response.Body.Close()

	if response.StatusCode == 304 { //Not Modified
		return TransferSkipped
	}

	//store some headers from the source to later identify whether this
	//resource has changed
	metadata = make(swift.Metadata)
	if etag := response.Header.Get("Etag"); etag != "" {
		metadata["source-etag"] = etag
	}
	if mtime := response.Header.Get("Last-Modified"); mtime != "" {
		metadata["source-last-modified"] = mtime
	}

	//upload file to target
	_, err = conn.ObjectPut(
		f.Job.TargetContainer,
		f.TargetObjectName(),
		response.Body,
		false, "",
		response.Header.Get("Content-Type"),
		metadata.ObjectHeaders(),
	)
	if err != nil {
		log.Printf("PUT %s/%s failed: %s", f.Job.TargetContainer, f.TargetObjectName(), err.Error())
		return TransferFailed
	}

	return TransferSuccess
}
