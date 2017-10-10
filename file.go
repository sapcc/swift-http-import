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
	"path/filepath"
	"strings"

	"github.com/ncw/swift"
	"github.com/sapcc/swift-http-import/pkg/objects"
	"github.com/sapcc/swift-http-import/pkg/util"
)

//File describes a single file which is mirrored as part of a Job.
type File struct {
	Job  *Job
	Path string
}

//TargetObjectName returns the object name of this file in the target container.
func (f File) TargetObjectName() string {
	objectName := strings.TrimPrefix(f.Path, "/")
	if f.Job.Target.ObjectNamePrefix == "" {
		return objectName
	}
	return filepath.Join(f.Job.Target.ObjectNamePrefix, objectName)
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
func (f File) PerformTransfer() TransferResult {
	//check if this file needs transfer
	if f.Job.ImmutableFileRx != nil && f.Job.ImmutableFileRx.MatchString(f.Path) {
		if f.Job.IsFileTransferred[f.TargetObjectName()] {
			util.Log(util.LogDebug, "skipping %s/%s: already transferred", f.Job.Target.ContainerName, f.TargetObjectName())
			return TransferSkipped
		}
	}

	util.Log(util.LogDebug, "transferring to %s/%s", f.Job.Target.ContainerName, f.TargetObjectName())

	//query the file metadata at the target
	_, headers, err := f.Job.Target.Connection.Object(
		f.Job.Target.ContainerName,
		f.TargetObjectName(),
	)
	if err != nil {
		if err == swift.ObjectNotFound {
			headers = swift.Headers{}
		} else {
			//log all other errors and skip the file (we don't want to waste
			//bandwidth downloading stuff if there is reasonable doubt that we will
			//not be able to upload it to Swift)
			util.Log(util.LogError, "skipping target %s/%s: HEAD failed: %s",
				f.Job.Target.ContainerName, f.TargetObjectName(),
				err.Error(),
			)
			return TransferFailed
		}
	}

	//retrieve object from source, taking advantage of Etag and Last-Modified where possible
	metadata := headers.ObjectMetadata()
	targetState := objects.FileState{
		Etag:         metadata["source-etag"],
		LastModified: metadata["source-last-modified"],
	}
	body, sourceState, err := f.Job.Source.GetFile(f.Path, targetState)
	if err != nil {
		util.Log(util.LogError, err.Error())
		return TransferFailed
	}
	if body != nil {
		defer body.Close()
	}
	if sourceState.SkipTransfer { // 304 Not Modified
		return TransferSkipped
	}

	//store some headers from the source to later identify whether this
	//resource has changed
	metadata = make(swift.Metadata)
	if sourceState.Etag != "" {
		metadata["source-etag"] = sourceState.Etag
	}
	if sourceState.LastModified != "" {
		metadata["source-last-modified"] = sourceState.LastModified
	}

	//upload file to target
	_, err = f.Job.Target.Connection.ObjectPut(
		f.Job.Target.ContainerName,
		f.TargetObjectName(),
		body,
		false, "",
		sourceState.ContentType,
		metadata.ObjectHeaders(),
	)
	if err != nil {
		util.Log(util.LogError, "PUT %s/%s failed: %s", f.Job.Target.ContainerName, f.TargetObjectName(), err.Error())

		//delete potentially incomplete upload
		err := f.Job.Target.Connection.ObjectDelete(
			f.Job.Target.ContainerName,
			f.TargetObjectName(),
		)
		if err != nil {
			util.Log(util.LogError, "DELETE %s/%s failed: %s", f.Job.Target.ContainerName, f.TargetObjectName(), err.Error())
		}

		return TransferFailed
	}

	return TransferSuccess
}
