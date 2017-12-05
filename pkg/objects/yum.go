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

package objects

import (
	"io"
)

//YumSource is a URLSource containing a Yum repository. This type reuses the
//Validate() and Connect() logic of URLSource, but adds a custom scraping
//implementation that reads the Yum repository metadata instead of relying on
//directory listings.
type YumSource URLSource

//Validate implements the Source interface.
func (s *YumSource) Validate(name string) []error {
	return (*URLSource)(s).Validate(name)
}

//Connect implements the Source interface.
func (s *YumSource) Connect() error {
	return (*URLSource)(s).Connect()
}

//ListAllFiles implements the Source interface.
func (s *YumSource) ListAllFiles() ([]string, *ListEntriesError) {
	//TODO
	return []string{"repodata/repomd.xml"}, nil
}

//ListEntries implements the Source interface.
func (s *YumSource) ListEntries(directoryPath string) ([]string, *ListEntriesError) {
	return nil, &ListEntriesError{
		Location: (*URLSource)(s).getURLForPath(directoryPath).String(),
		Message:  "ListEntries is not implemented for YumSource",
	}
}

//GetFile implements the Source interface.
func (s *YumSource) GetFile(directoryPath string, targetState FileState) (body io.ReadCloser, sourceState FileState, err error) {
	return (*URLSource)(s).GetFile(directoryPath, targetState)
}
