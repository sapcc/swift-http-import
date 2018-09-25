/******************************************************************************
*
*  Copyright 2018 Stefan Majewsky <majewsky@gmx.net>
*
*  Licensed under the Apache License, Version 2.0 (the "License");
*  you may not use this file except in compliance with the License.
*  You may obtain a copy of the License at
*
*      http://www.apache.org/licenses/LICENSE-2.0
*
*  Unless required by applicable law or agreed to in writing, software
*  distributed under the License is distributed on an "AS IS" BASIS,
*  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
*  See the License for the specific language governing permissions and
*  limitations under the License.
*
******************************************************************************/

package schwift

import (
	"net/http"
	"testing"
	"time"
)

type tempurlBogusBackend struct{}

func (tempurlBogusBackend) EndpointURL() string {
	return "https://example.com/v1/AUTH_example/"
}
func (tempurlBogusBackend) Clone(newEndpointURL string) Backend {
	panic("unimplemented")
}
func (tempurlBogusBackend) Do(req *http.Request) (*http.Response, error) {
	panic("unimplemented")
}

func TestObjectTempURL(t *testing.T) {
	//setup a bogus backend, account, container and object with exact names to
	//reproducibly generate a temp URL
	account, err := InitializeAccount(tempurlBogusBackend{})
	if err != nil {
		t.Fatal(err.Error())
	}

	actualURL, err := account.Container("foo").Object("bar").TempURL("supersecretkey", "GET", time.Unix(1e9, 0))
	if err != nil {
		t.Fatal(err.Error())
	}

	expectedURL := "https://example.com/v1/AUTH_example/foo/bar?temp_url_sig=ed44d92005345aee463c884d76d4850ef6d2778d&temp_url_expires=1000000000"
	if actualURL != expectedURL {
		t.Error("temp URL generation failed")
		t.Logf("expected: %s\n", expectedURL)
		t.Logf("actual: %s\n", actualURL)
	}
}
