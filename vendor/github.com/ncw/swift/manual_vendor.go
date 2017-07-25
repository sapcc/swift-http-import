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

// NOTE: I added this file to the vendor/ directory manually because
// https://github.com/ncw/swift/pull/94 is not yet merged. Do not delete it
// until the pull request is merged, and the pin for github.com/ncw/swift has
// been updated to include the merged PR.

package swift

//StatusCode returns the HTTP status code that Swift returned when this file
//was opened for reading. If at least one of the If-None-Match or
//If-Last-Modified headers were specified during opening, this code may be 304
//to indicate that the file was not modified, in which case Read() will not
//return any bytes.
func (file *ObjectOpenFile) StatusCode() int {
	return file.resp.StatusCode
}
