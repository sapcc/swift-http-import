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
	"io"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/majewsky/schwift"
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
	//only set for symlinks (refers to a path below the ObjectPrefix in the same container)
	SymlinkTargetPath string
	//results of GET on this file
	Contents []byte
	Headers  http.Header
}

//TargetObject returns the object corresponding to this file in the target container.
func (f File) TargetObject() *schwift.Object {
	return f.Job.Target.ObjectAtPath(f.Spec.Path)
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
	object := f.TargetObject()

	//check if this file needs transfer
	if f.Job.Matcher.ImmutableFileRx != nil && f.Job.Matcher.ImmutableFileRx.MatchString(f.Spec.Path) {
		if f.Job.Target.FileExists[object.Name()] {
			util.Log(util.LogDebug, "skipping %s: already transferred", object.FullName())
			return TransferSkipped
		}
	}

	//can only transfer as a symlink if the target server supports it
	capabilities, err := f.Job.Target.Container.Account().Capabilities()
	if err != nil {
		util.Log(util.LogFatal, "query /info on target failed: %s", err.Error())
	}
	if capabilities.Symlink == nil {
		f.Spec.SymlinkTargetPath = ""
	}

	//symlinks are safe to use only if the target object is also included in this job
	//(TODO extend validation to allow for target to be transferred by any job,
	//e.g. by adding a new actor between scraper and transferor that has access
	//to the full list of jobs)
	if f.Spec.SymlinkTargetPath != "" {
		if f.Job.Matcher.CheckRecursive(f.Spec.SymlinkTargetPath) != nil {
			f.Spec.SymlinkTargetPath = ""
		}
	}

	util.Log(util.LogDebug, "transferring to %s", object.FullName())

	//query the file metadata at the target
	hdr, currentSymlinkTarget, err := object.SymlinkHeaders()
	if err != nil {
		if schwift.Is(err, http.StatusNotFound) {
			hdr = schwift.NewObjectHeaders()
			currentSymlinkTarget = nil
		} else {
			//log all other errors and skip the file (we don't want to waste
			//bandwidth downloading stuff if there is reasonable doubt that we will
			//not be able to upload it to Swift)
			util.Log(util.LogError, "skipping target %s: HEAD failed: %s",
				object.FullName(), err.Error(),
			)
			return TransferFailed
		}
	}

	//if we want to upload a symlink, we can skip the whole Last-Modified/Etag
	//shebang and straight-up compare the symlink target
	if f.Spec.SymlinkTargetPath != "" {
		return f.uploadSymlink(currentSymlinkTarget, hdr.IsLargeObject())
	}

	//retrieve object from source, taking advantage of Etag and Last-Modified where possible
	metadata := hdr.Metadata()
	requestHeaders := schwift.NewObjectHeaders()
	if val := metadata.Get("Source-Etag"); val != "" {
		requestHeaders.Set("If-None-Match", val)
	}
	if val := metadata.Get("Source-Last-Modified"); val != "" {
		requestHeaders.Set("If-Modified-Since", val)
	}

	var (
		body        io.ReadCloser
		sourceState FileState
	)
	if f.Spec.Contents == nil {
		body, sourceState, err = f.Job.Source.GetFile(f.Spec.Path, requestHeaders)
	} else {
		util.Log(util.LogDebug, "using cached contents for %s", f.Spec.Path)
		body, sourceState, err = f.Spec.toTransferFormat(requestHeaders)
	}
	if err != nil {
		util.Log(util.LogError, "GET %s failed: %s", f.Spec.Path, err.Error())
		return TransferFailed
	}
	if body != nil {
		defer body.Close()
	}
	if sourceState.SkipTransfer { // 304 Not Modified
		return TransferSkipped
	}

	if util.LogIndividualTransfers {
		util.Log(util.LogInfo, "transferring to %s", object.FullName())
	}

	//store some headers from the source to later identify whether this
	//resource has changed
	uploadHeaders := schwift.NewObjectHeaders()
	uploadHeaders.ContentType().Set(sourceState.ContentType)
	if sourceState.Etag != "" {
		uploadHeaders.Metadata().Set("Source-Etag", sourceState.Etag)
	}
	if sourceState.LastModified != "" {
		uploadHeaders.Metadata().Set("Source-Last-Modified", sourceState.LastModified)
	}
	if f.Job.Expiration.Enabled && sourceState.ExpiryTime != nil {
		delay := time.Duration(f.Job.Expiration.DelaySeconds) * time.Second
		uploadHeaders.ExpiresAt().Set(sourceState.ExpiryTime.Add(delay))
	}

	//upload file to target
	var ok bool
	size := sourceState.SizeBytes
	if f.Job.Segmenting != nil && size > 0 && uint64(size) >= f.Job.Segmenting.MinObjectSize {
		ok = f.uploadLargeObject(body, uploadHeaders, hdr.IsLargeObject())
	} else {
		ok = f.uploadNormalObject(body, uploadHeaders, hdr.IsLargeObject())
	}

	if ok {
		return TransferSuccess
	}
	return TransferFailed
}

func (f File) uploadSymlink(previousTarget *schwift.Object, cleanupOldSegments bool) TransferResult {
	object := f.TargetObject()
	newTarget := f.Job.Target.ObjectAtPath(f.Spec.SymlinkTargetPath)

	if previousTarget != nil && newTarget.IsEqualTo(previousTarget) {
		util.Log(util.LogDebug, "skipping %s: already symlinked to the correct target", object.FullName())
		return TransferSkipped
	}

	err := object.SymlinkTo(newTarget, &schwift.SymlinkOptions{
		DeleteSegments: cleanupOldSegments,
	}, nil)
	if err == nil {
		return TransferSuccess
	}

	cleanupFailedUpload(object)
	return TransferFailed
}

func (s FileSpec) toTransferFormat(requestHeaders schwift.ObjectHeaders) (io.ReadCloser, FileState, error) {
	targetState := FileState{
		Etag:         requestHeaders.Get("If-None-Match"),
		LastModified: requestHeaders.Get("If-Modified-Since"),
	}

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

//StatusSwiftRateLimit is the non-standard HTTP status code used by Swift to
//indicate Too Many Requests.
const StatusSwiftRateLimit = 498

func (f File) uploadNormalObject(body io.Reader, hdr schwift.ObjectHeaders, cleanupOldSegments bool) (ok bool) {
	object := f.TargetObject()
	err := object.Upload(body, &schwift.UploadOptions{
		DeleteSegments: cleanupOldSegments,
	}, hdr.ToOpts())
	if err == nil {
		return true
	}

	util.Log(util.LogError, "PUT %s failed: %s", object.FullName(), err.Error())

	if schwift.Is(err, StatusSwiftRateLimit) {
		//upload failed due to rate limit, object is definitely not uploaded
		//prevent additional rate limit caused by an unnecessary delete request
		return false
	}

	cleanupFailedUpload(object)
	return false
}

func (f File) uploadLargeObject(body io.Reader, hdr schwift.ObjectHeaders, cleanupOldSegments bool) (ok bool) {
	object := f.TargetObject()

	lo, err := object.AsNewLargeObject(schwift.SegmentingOptions{
		SegmentContainer: f.Job.Segmenting.Container,
		Strategy:         schwift.StaticLargeObject,
	}, &schwift.TruncateOptions{
		DeleteSegments: cleanupOldSegments,
	})
	if err == nil {
		XDeleteAtHeader := schwift.NewObjectHeaders()
		if hdr.ExpiresAt().Exists() {
			XDeleteAtHeader.ExpiresAt().Set(hdr.ExpiresAt().Get())
		}
		err = lo.Append(body, int64(f.Job.Segmenting.SegmentSize), XDeleteAtHeader.ToOpts())
	}
	if err == nil {
		err = lo.WriteManifest(hdr.ToOpts())
	}
	if err == nil {
		util.Log(util.LogInfo, "PUT %s has created a Static Large Object with segments in %s/%s/",
			object.FullName(), lo.SegmentContainer().Name(), lo.SegmentPrefix(),
		)
		return true
	}

	util.Log(util.LogError, "PUT %s as Static Large Object failed: %s", object.FullName(), err.Error())

	//file was not transferred correctly - cleanup manifest and segments
	cleanupFailedUpload(object)
	return false
}

func cleanupFailedUpload(object *schwift.Object) {
	//file was not transferred correctly - cleanup manifest and segments
	err := object.Delete(&schwift.DeleteOptions{
		DeleteSegments: true,
	}, nil)
	if err != nil && !schwift.Is(err, http.StatusNotFound) {
		util.Log(util.LogError, "DELETE %s failed: %s", object.FullName(), err.Error())
	}
}
