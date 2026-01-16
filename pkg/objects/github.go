// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package objects

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/majewsky/schwift/v2"
	"github.com/sapcc/go-api-declarations/bininfo"
	"github.com/sapcc/go-bits/regexpext"
	"github.com/sapcc/go-bits/secrets"

	"github.com/sapcc/swift-http-import/pkg/util"
)

type GithubReleaseSource struct {
	// Options from config file.
	URLString         string                `yaml:"url"`
	Token             secrets.FromEnv       `yaml:"token"`
	TagNamePattern    regexpext.PlainRegexp `yaml:"tag_name_pattern"`
	IncludeDraft      bool                  `yaml:"include_draft"`
	IncludePrerelease bool                  `yaml:"include_prerelease"`

	// Compiled configuration.
	repoURL            *url.URL `yaml:"-"`
	releaseEndpointURL *url.URL `yaml:"-"`
	// notOlderThan is used to limit release listing to prevent excess API requests.
	notOlderThan *time.Time `yaml:"-"`
}

// githubRepoRx is used to extract repository owner and name from a url.URL.Path field.
//
// Example:
//
//	Input: /sapcc/swift-http-import
//	Match groups: ["sapcc", "swift-http-import"]
var githubRepoRx = regexp.MustCompile(`^/([^\s/]+)/([^\s/]+)/?$`)

// Validate implements the Source interface.
func (s *GithubReleaseSource) Validate(name string) []error {
	var err error
	s.repoURL, err = url.Parse(s.URLString)
	if err != nil {
		return []error{fmt.Errorf("could not parse %s.url: %w", name, err)}
	}

	// validate s.repoURL
	errInvalidURL := fmt.Errorf("invalid value for %s.url: expected a url in the format %q, got: %q",
		name, "http(s)://<hostname>/<owner>/<repo>", s.URLString)
	if s.repoURL.Scheme != "http" && s.repoURL.Scheme != "https" {
		return []error{errInvalidURL}
	}
	if s.repoURL.RawQuery != "" || s.repoURL.Fragment != "" {
		return []error{errInvalidURL}
	}
	match := githubRepoRx.FindStringSubmatch(s.repoURL.Path)
	if match == nil {
		return []error{errInvalidURL}
	}
	ownerName, repoName := match[1], match[2]

	// derive apiBaseURL from s.repoURL
	var apiBaseURL *url.URL
	if s.repoURL.Hostname() == "github.com" {
		apiBaseURL, err = url.Parse("https://api.github.com/")
		if err != nil {
			return []error{fmt.Errorf("could not build apiBaseURL: %w", err)}
		}
	} else {
		repoURLCloned := *s.repoURL
		repoURLCloned.Path = "/api/v3/"
		repoURLCloned.RawPath = "/api/v3/"
		apiBaseURL = &repoURLCloned
	}

	// derive endpoint URL for release listing
	// (this sets a higher page size than the default of 30 to avoid exceeding the API rate limit)
	const pageSize = 50
	endpointPath := fmt.Sprintf("repos/%s/%s/releases?per_page=%d", ownerName, repoName, pageSize)
	s.releaseEndpointURL, err = apiBaseURL.Parse(endpointPath)
	if err != nil {
		return []error{fmt.Errorf("could not build URL for releases of %s: %w", s.repoURL.String(), err)}
	}

	// validate s.Token
	if s.repoURL.Hostname() != "github.com" {
		if s.Token == "" {
			return []error{fmt.Errorf("%s.token is required for repositories hosted on GitHub Enterprise", name)}
		}
	}

	return nil
}

// Connect implements the Source interface.
func (s *GithubReleaseSource) Connect(ctx context.Context, name string) error {
	return nil
}

// ListEntries implements the Source interface.
func (s *GithubReleaseSource) ListEntries(_ context.Context, directoryPath string) ([]FileSpec, *ListEntriesError) {
	return nil, ErrListEntriesNotSupported
}

// ListAllFiles implements the Source interface.
func (s *GithubReleaseSource) ListAllFiles(ctx context.Context, out chan<- FileSpec) *ListEntriesError {
	releases, err := s.getReleases(ctx)
	if err != nil {
		return &ListEntriesError{
			Location: s.repoURL.String(),
			Message:  "could not list releases",
			Inner:    err,
		}
	}

	for _, r := range releases {
		if !s.IncludeDraft && r.IsDraft {
			continue
		}
		if !s.IncludePrerelease && r.IsPrerelease {
			continue
		}
		if !s.TagNamePattern.MatchString(r.TagName) {
			continue
		}

		for _, a := range r.Assets {
			fs := FileSpec{
				Path:         fmt.Sprintf("%s/%s", r.TagName, a.Name),
				DownloadPath: a.DownloadURL,
				LastModified: util.PointerTo(a.UpdatedAt),
			}
			out <- fs
		}
	}

	return nil
}

const (
	githubHeaderAPIVersion  = "X-Github-Api-Version"
	githubDefaultAPIVersion = "2022-11-28"
	githubMediaType         = "application/vnd.github.v3+json"
)

// GetFile implements the Source interface.
func (s *GithubReleaseSource) GetFile(ctx context.Context, path string, requestHeaders schwift.ObjectHeaders) (io.ReadCloser, FileState, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, path, http.NoBody)
	if err != nil {
		return nil, FileState{}, fmt.Errorf("skipping: could not create request for %s: %w", path, err)
	}
	for key, val := range requestHeaders.Headers {
		req.Header.Set(key, val)
	}
	req.Header.Set(githubHeaderAPIVersion, githubDefaultAPIVersion)
	req.Header.Set("User-Agent", "swift-http-import/"+bininfo.VersionOr("dev"))
	req.Header.Set("Accept", "application/octet-stream")
	if s.Token != "" {
		req.Header.Set("Authorization", "Bearer "+string(s.Token))
	}

	// We use http.DefaultClient explicitly instead of retrieving (s.client.Client()) the
	// same http.Client that was passed to github.Client because that http.Client, when
	// obtained using oauth2.NewClient(), does not return all headers in the request
	// response.
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, FileState{}, fmt.Errorf("skipping %s: GET failed: %w", req.URL.String(), err)
	}

	if resp.StatusCode != http.StatusOK &&
		resp.StatusCode != http.StatusNotModified {
		return nil, FileState{}, fmt.Errorf(
			"skipping %s: GET returned unexpected status code: expected 200 or 304, but got %d",
			req.URL.String(), resp.StatusCode,
		)
	}

	var sizeBytes *uint64
	if resp.ContentLength < 0 {
		sizeBytes = nil
	} else {
		sizeBytes = util.PointerTo(util.AtLeastZero(resp.ContentLength))
	}

	return resp.Body, FileState{
		Etag:         resp.Header.Get("Etag"),
		LastModified: resp.Header.Get("Last-Modified"),
		SizeBytes:    sizeBytes,
		ExpiryTime:   nil, // no way to get this information via HTTP only
		SkipTransfer: resp.StatusCode == http.StatusNotModified,
		ContentType:  resp.Header.Get("Content-Type"),
	}, nil
}

type githubRelease struct {
	TagName      string    `json:"tag_name"`
	IsDraft      bool      `json:"draft"`
	IsPrerelease bool      `json:"prerelease"`
	PublishedAt  time.Time `json:"published_at"`
	Assets       []struct {
		DownloadURL string    `json:"url"`
		Name        string    `json:"name"`
		UpdatedAt   time.Time `json:"updated_at"`
	} `json:"assets"`
}

func (s *GithubReleaseSource) getReleases(ctx context.Context) ([]githubRelease, error) {
	var result []githubRelease

	endpointURLString := s.releaseEndpointURL.String()
	for endpointURLString != "" {
		// build request
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpointURLString, http.NoBody)
		if err != nil {
			return nil, fmt.Errorf("could not create request for %s: %w", endpointURLString, err)
		}
		req.Header.Set(githubHeaderAPIVersion, githubDefaultAPIVersion)
		req.Header.Set("User-Agent", "swift-http-import/"+bininfo.VersionOr("dev"))
		req.Header.Set("Accept", githubMediaType)
		if s.Token != "" {
			req.Header.Set("Authorization", "Bearer "+string(s.Token))
		}

		// execute request
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("could not GET %s: %w", endpointURLString, err)
		}
		defer resp.Body.Close()

		// expect status code 200 OK
		if resp.StatusCode != http.StatusOK {
			var respBody string
			buf, err := io.ReadAll(resp.Body)
			if err == nil {
				respBody = string(buf)
			} else {
				respBody = "could not read body: " + err.Error()
			}
			return nil, fmt.Errorf("could not GET %s: expected 200 OK, but got %s (response was: %s)", endpointURLString, resp.Status, respBody)
		}

		// decode response body
		var page []githubRelease
		err = json.NewDecoder(resp.Body).Decode(&page)
		if err != nil {
			return nil, fmt.Errorf("could not GET %s: while parsing JSON response body: %w", endpointURLString, err)
		}
		result = append(result, page...)

		// Check if the last release in the result slice is newer than the notOlderThan
		// time. If not, then we don't need to get further releases.
		if s.notOlderThan != nil {
			lastRelease := result[len(result)-1]
			if s.notOlderThan.After(lastRelease.PublishedAt) {
				break
			}
		}

		// URL for next page is in `Link` header, looking like `<https://...>; rel="next"`
		endpointURLString = "" // if we do not find one below, we are on the last page and need to break the loop
		if linkHeader := resp.Header.Get("Link"); linkHeader != "" {
			for link := range strings.SplitSeq(linkHeader, ",") {
				href, metadata, ok := strings.Cut(strings.TrimSpace(link), ";")
				if !ok {
					continue
				}
				if strings.TrimSpace(metadata) != `rel="next"` {
					continue
				}

				href, ok = strings.CutPrefix(href, "<")
				if !ok {
					continue
				}
				href, ok = strings.CutSuffix(href, ">")
				if !ok {
					continue
				}

				endpointURLString = href
				break
			}
		}
	}

	return result, nil
}
