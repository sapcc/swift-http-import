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
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"regexp"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/sapcc/swift-http-import/pkg/util"
)

//Source describes a place from which files can be fetched.
type Source interface {
	//Validate reports errors if this source is malspecified.
	Validate(name string) []error
	//Connect performs source-specific one-time setup.
	Connect() error
	//ListEntries returns all files and subdirectories at this path in the
	//source. Each result value must have a "/" prefix for subdirectories, or
	//none for files.
	ListEntries(path string) ([]string, *ListEntriesError)
	//GetFile retrieves the contents and metadata for the file at the given path
	//in the source.
	GetFile(path string, targetState FileState) (body io.ReadCloser, sourceState FileState, err error)
}

//ListEntriesError is an error that occurs while scraping a directory.
type ListEntriesError struct {
	//the location of the directory (e.g. an URL)
	Location string
	//error message
	Message string
}

//FileState is used by Source.GetFile() to describe the state of a file.
type FileState struct {
	Etag         string
	LastModified string
	//the following fields are only used in `sourceState`, not `targetState`
	SkipTransfer bool
	ContentType  string
}

////////////////////////////////////////////////////////////////////////////////

//URLSource describes a source that's accessible via HTTP.
type URLSource struct {
	URL string `yaml:"url"`
	//auth options
	ClientCertificatePath    string       `yaml:"cert"`
	ClientCertificateKeyPath string       `yaml:"key"`
	ServerCAPath             string       `yaml:"ca"`
	HTTPClient               *http.Client `yaml:"-"`
}

//Validate implements the Source interface.
func (u *URLSource) Validate(name string) (result []error) {
	if u.URL == "" {
		result = append(result, fmt.Errorf("missing value for %s.url", name))
	}

	// If one of the following is set, the other one needs also to be set
	if u.ClientCertificatePath != "" || u.ClientCertificateKeyPath != "" {
		if u.ClientCertificatePath == "" {
			result = append(result, fmt.Errorf("missing value for %s.cert", name))
		}
		if u.ClientCertificateKeyPath == "" {
			result = append(result, fmt.Errorf("missing value for %s.key", name))
		}
	}

	return
}

//Connect implements the Source interface.
func (u *URLSource) Connect() error {
	tlsConfig := &tls.Config{}

	if u.ClientCertificatePath != "" {
		// Load client cert
		clientCertificate, err := tls.LoadX509KeyPair(u.ClientCertificatePath, u.ClientCertificateKeyPath)
		if err != nil {
			return fmt.Errorf("cannot load client certificate from %s: %s", u.ClientCertificatePath, err.Error())
		}

		util.Log(util.LogDebug, "Client certificate %s loaded", u.ClientCertificatePath)
		tlsConfig.Certificates = []tls.Certificate{clientCertificate}
	}

	if u.ServerCAPath != "" {
		// Load server CA cert
		serverCA, err := ioutil.ReadFile(u.ServerCAPath)
		if err != nil {
			return fmt.Errorf("cannot load CA certificate from %s: %s", u.ServerCAPath, err.Error())
		}

		certPool := x509.NewCertPool()
		certPool.AppendCertsFromPEM(serverCA)

		util.Log(util.LogDebug, "Server CA %s loaded", u.ServerCAPath)
		tlsConfig.RootCAs = certPool
	}

	if u.ClientCertificatePath != "" || u.ServerCAPath != "" {
		tlsConfig.BuildNameToCertificate()
		// Overriding the transport for TLS, requires also Proxy to be set from ENV,
		// otherwise a set proxy will get lost
		transport := &http.Transport{TLSClientConfig: tlsConfig, Proxy: http.ProxyFromEnvironment}
		u.HTTPClient = &http.Client{Transport: transport}
	} else {
		u.HTTPClient = http.DefaultClient
	}

	return nil
}

//matches scheme prefix (e.g. "http:" or "git+ssh:") at the start of a full URL
//[RFC 3986, 3.1]
var schemeRx = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*:`)

//matches ".." path element
var dotdotRx = regexp.MustCompile(`(?:^|/)\.\.(?:$|/)`)

//ListEntries implements the Source interface.
func (u URLSource) ListEntries(path string) ([]string, *ListEntriesError) {
	//get full URL of this subdirectory
	url := u.getURLForPath(path)
	//to get a well-formatted directory listing, the directory URL must have a
	//trailing slash (most web servers automatically redirect from the URL
	//without trailing slash to the URL with trailing slash; others show a
	//slightly different directory listing that we cannot parse correctly)
	if !strings.HasSuffix(url, "/") {
		url += "/"
	}

	util.Log(util.LogDebug, "scraping %s", url)

	//retrieve directory listing
	//TODO: This should send "Accept: text/html", but at least Apache and nginx
	//don't care about the Accept header, anyway, as far as my testing showed.
	response, err := u.HTTPClient.Get(url)
	if err != nil {
		return nil, &ListEntriesError{url, "GET failed: " + err.Error()}
	}
	defer response.Body.Close()

	//check that we actually got a directory listing
	if !strings.HasPrefix(response.Status, "2") {
		return nil, &ListEntriesError{url, "GET returned status " + response.Status}
	}
	contentType := response.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "text/html") {
		return nil, &ListEntriesError{url, "GET returned unexpected Content-Type: " + contentType}
	}

	//find links inside the HTML document
	tokenizer := html.NewTokenizer(response.Body)
	var result []string
	for {
		tokenType := tokenizer.Next()

		switch tokenType {
		case html.ErrorToken:
			//end of document
			return result, nil
		case html.StartTagToken:
			token := tokenizer.Token()

			if token.DataAtom == atom.A {
				//found an <a> tag -- retrieve its href
				var href string
				for _, attr := range token.Attr {
					if attr.Key == "href" {
						href = attr.Val
						break
					}
				}
				if href == "" {
					continue
				}

				//filter external links with full URLs
				if schemeRx.MatchString(href) {
					continue
				}
				//links with trailing slashes are absolute paths as well; either to
				//another server, e.g. "//ajax.googleapis.com/jquery.js", or to the
				//toplevel of this server, e.g. "/static/site.css")
				if strings.HasPrefix(href, "/") {
					continue
				}
				//links with ".." path elements cannot be guaranteed to be pointing to a
				//resource below this directory, so skip them as well (this assumes that
				//the sender did already clean his relative links so that no ".." appears
				//in legitimate downward links)
				if dotdotRx.MatchString(href) {
					continue
				}
				//ignore links with a query part (Apache directory listings use these for
				//adjustable sorting)
				if strings.Contains(href, "?") {
					continue
				}

				result = append(result, href)
			}
		}
	}
}

//GetFile implements the Source interface.
func (u URLSource) GetFile(path string, targetState FileState) (io.ReadCloser, FileState, error) {
	url := u.getURLForPath(path)

	//prepare request to retrieve from source, taking advantage of Etag and
	//Last-Modified where possible
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, FileState{}, fmt.Errorf("skipping %s: GET failed: %s", url, err.Error())
	}
	if targetState.Etag != "" {
		req.Header.Set("If-None-Match", targetState.Etag)
	}
	if targetState.LastModified != "" {
		req.Header.Set("If-Modified-Since", targetState.LastModified)
	}

	//retrieve file from source
	response, err := u.HTTPClient.Do(req)
	if err != nil {
		return nil, FileState{}, fmt.Errorf("skipping %s: GET failed: %s", url, err.Error())
	}
	if response.StatusCode != 200 && response.StatusCode != 304 {
		return nil, FileState{}, fmt.Errorf(
			"skipping %s: GET returned unexpected status code: expected 200 or 304, but got %d",
			url, response.StatusCode,
		)
	}

	return response.Body, FileState{
		Etag:         response.Header.Get("Etag"),
		LastModified: response.Header.Get("Last-Modified"),
		SkipTransfer: response.StatusCode == 304,
		ContentType:  response.Header.Get("Content-Type"),
	}, nil
}

//Return the URL for the given path below this URLSource.
func (u URLSource) getURLForPath(path string) string {
	result := u.URL
	if !strings.HasSuffix(result, "/") {
		result += "/"
	}

	return result + strings.TrimPrefix(path, "/")
}
