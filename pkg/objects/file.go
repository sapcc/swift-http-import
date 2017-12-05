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
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ncw/swift"
	"github.com/sapcc/swift-http-import/pkg/util"
)

//File describes a single file which is mirrored as part of a Job.
type File struct {
	Job  *Job
	Spec FileSpec
}

//FileSpec contains metadata for a File. The only required field is Path.
//Sources that download some files early (during scraping) can pass the
//downloaded contents and metadata in the remaining fields of the FileSpec to
//avoid double download.
type FileSpec struct {
	Path        string
	IsDirectory bool
	//results of GET on this file
	Contents []byte
	Headers  http.Header
}

//TargetObjectName returns the object name of this file in the target container.
func (f File) TargetObjectName() string {
	objectName := strings.TrimPrefix(f.Spec.Path, "/")
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
	if f.Job.Matcher.ImmutableFileRx != nil && f.Job.Matcher.ImmutableFileRx.MatchString(f.Spec.Path) {
		if f.Job.Target.FileExists[f.TargetObjectName()] {
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
	targetState := FileState{
		Etag:         metadata["source-etag"],
		LastModified: metadata["source-last-modified"],
	}

	var (
		body        io.ReadCloser
		sourceState FileState
	)
	if f.Spec.Contents == nil {
		body, sourceState, err = f.Job.Source.GetFile(f.Spec.Path, targetState)
	} else {
		util.Log(util.LogDebug, "using cached contents for %s", f.Spec.Path)
		body, sourceState, err = f.Spec.toTransferFormat(targetState)
	}
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

	if util.LogIndividualTransfers {
		util.Log(util.LogInfo, "transferring to %s/%s", f.Job.Target.ContainerName, f.TargetObjectName())
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
	headers = metadata.ObjectHeaders()
	if f.Job.Expiration.Enabled && sourceState.ExpiryTime != nil {
		delay := int64(f.Job.Expiration.DelaySeconds)
		headers["X-Delete-At"] = strconv.FormatInt(sourceState.ExpiryTime.Unix()+delay, 10)
	}

	//upload file to target
	var ok bool
	size := sourceState.SizeBytes
	if f.Job.Segmenting != nil && size > 0 && uint64(size) >= f.Job.Segmenting.MinObjectSize {
		ok = f.uploadLargeObject(body, sourceState, headers)
	} else {
		ok = f.uploadNormalObject(body, sourceState, headers)
	}

	if ok {
		return TransferSuccess
	}
	return TransferFailed
}

func (s FileSpec) toTransferFormat(targetState FileState) (io.ReadCloser, FileState, error) {
	sourceState := FileState{
		Etag:         s.Headers.Get("Etag"),
		LastModified: s.Headers.Get("Last-Modified"),
		SizeBytes:    int64(len(s.Contents)),
		ExpiryTime:   nil,
		ContentType:  s.Headers.Get("Content-Type"),
	}

	if targetState.Etag != "" && sourceState.Etag != "" {
		sourceState.SkipTransfer = targetState.Etag == sourceState.Etag
	} else if targetState.LastModified != "" && sourceState.LastModified != "" {
		//need to parse Last-Modified timestamps to compare between target and source
		targetMtime, err := http.ParseTime(targetState.LastModified)
		if err != nil {
			return nil, sourceState, err
		}
		sourceMtime, err := http.ParseTime(sourceState.LastModified)
		if err != nil {
			return nil, sourceState, err
		}
		sourceState.SkipTransfer = targetMtime.Equal(sourceMtime)
	}

	return ioutil.NopCloser(bytes.NewReader(s.Contents)), sourceState, nil
}

func (f File) uploadNormalObject(body io.Reader, sourceState FileState, hdr swift.Headers) (ok bool) {
	containerName := f.Job.Target.ContainerName
	objectName := f.TargetObjectName()
	_, err := f.Job.Target.Connection.ObjectPut(
		containerName, objectName,
		body,
		false, "",
		sourceState.ContentType,
		hdr,
	)
	if err == nil {
		return true
	}

	util.Log(util.LogError, "PUT %s/%s failed: %s", containerName, objectName, err.Error())

	if serr, ok := err.(*swift.Error); ok {
		//upload failed due to rate limit, object is definitely not uploaded
		//prevent additional rate limit caused by an unnecessary delete request
		if serr.StatusCode == 498 {
			return false
		}
	}

	//delete potentially incomplete upload
	err = f.Job.Target.Connection.ObjectDelete(containerName, objectName)
	if err != nil {
		util.Log(util.LogError, "DELETE %s/%s failed: %s", containerName, objectName, err.Error())
	}

	return false
}

func (f File) uploadLargeObject(body io.Reader, sourceState FileState, hdr swift.Headers) (ok bool) {
	containerName := f.Job.Target.ContainerName
	objectName := f.TargetObjectName()
	segmentContainerName := f.Job.Segmenting.ContainerName

	now := time.Now()
	segmentPrefix := fmt.Sprintf("%s/slo/%d.%09d/%d/%d",
		objectName, now.Unix(), now.Nanosecond(), sourceState.SizeBytes, f.Job.Segmenting.SegmentSize,
	)

	largeObj, err := f.Job.Target.Connection.StaticLargeObjectCreate(&swift.LargeObjectOpts{
		Container:        containerName,
		ObjectName:       objectName,
		ContentType:      sourceState.ContentType,
		Headers:          hdr,
		ChunkSize:        int64(f.Job.Segmenting.SegmentSize),
		SegmentContainer: segmentContainerName,
		SegmentPrefix:    segmentPrefix,
	})
	if err == nil {
		_, err = io.CopyBuffer(largeObj, body, make([]byte, 1<<20))
	}
	if err == nil {
		err = largeObj.Close()
	}
	if err == nil {
		util.Log(util.LogInfo, "PUT %s/%s has created a Static Large Object with segments in %s/%s/",
			containerName, objectName, segmentContainerName, segmentPrefix,
		)
		return true
	}

	util.Log(util.LogError, "PUT %s/%s as Static Large Object failed: %s", containerName, objectName, err.Error())

	//file was not transferred correctly - cleanup manifest...
	err = f.Job.Target.Connection.ObjectDelete(containerName, objectName)
	if err != nil && err != swift.ObjectNotFound {
		util.Log(util.LogError, "DELETE %s/%s failed: %s", containerName, objectName, err.Error())
	}
	//...and segments
	segmentNames, err := f.Job.Target.Connection.ObjectNamesAll(segmentContainerName,
		&swift.ObjectsOpts{Prefix: segmentPrefix + "/"},
	)
	if err != nil {
		util.Log(util.LogError, "cannot enumerate SLO segments in %s/%s/ for cleanup: %s",
			segmentContainerName, segmentPrefix, err.Error(),
		)
		segmentNames = nil
	}
	if len(segmentNames) > 0 {
		result, err := f.Job.Target.Connection.BulkDelete(segmentContainerName, segmentNames)
		if err != nil {
			util.Log(util.LogError, "DELETE %s/%s/* failed: %s", segmentContainerName, segmentPrefix, err.Error())
		} else {
			for segmentName, err := range result.Errors {
				util.Log(util.LogError, "DELETE %s/%s failed: %s", segmentContainerName, segmentName, err.Error())
			}
		}
	}

	return false
}
