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

package tests

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/majewsky/schwift"
)

var objectExampleContent = []byte(`{"message":"Hello World!"}`)
var objectExampleContentEtag = etagOf(objectExampleContent)

func TestObjectIterator(t *testing.T) {
	testWithContainer(t, func(c *schwift.Container) {
		oname := func(idx int) string {
			return fmt.Sprintf("schwift-test-listing%d", idx)
		}

		//create test objects that can be listed
		for idx := 1; idx <= 4; idx++ {
			hdr := schwift.NewObjectHeaders()
			hdr.ContentType().Set("application/json")
			err := c.Object(oname(idx)).Upload(bytes.NewReader(objectExampleContent), nil, hdr.ToOpts())
			expectSuccess(t, err)
		}

		//test iteration with empty last page
		iter := c.Objects()
		iter.Prefix = "schwift-test-listing"
		os, err := iter.NextPage(2)
		expectSuccess(t, err)
		expectObjectNames(t, os, oname(1), oname(2))
		os, err = iter.NextPage(2)
		expectSuccess(t, err)
		expectObjectNames(t, os, oname(3), oname(4))
		os, err = iter.NextPage(2)
		expectSuccess(t, err)
		expectObjectNames(t, os)
		os, err = iter.NextPage(2)
		expectSuccess(t, err)
		expectObjectNames(t, os)

		//test iteration with partial last page
		iter = c.Objects()
		iter.Prefix = "schwift-test-listing"
		os, err = iter.NextPage(3)
		expectSuccess(t, err)
		expectObjectNames(t, os, oname(1), oname(2), oname(3))
		os, err = iter.NextPage(3)
		expectSuccess(t, err)
		expectObjectNames(t, os, oname(4))
		os, err = iter.NextPage(4)
		expectSuccess(t, err)
		expectObjectNames(t, os)

		//test detailed iteration
		iter = c.Objects()
		iter.Prefix = "schwift-test-listing"
		ois, err := iter.NextPageDetailed(2)
		expectSuccess(t, err)
		expectObjectInfos(t, ois, oname(1), oname(2))
		ois, err = iter.NextPageDetailed(3)
		expectSuccess(t, err)
		expectObjectInfos(t, ois, oname(3), oname(4))
		ois, err = iter.NextPageDetailed(3)
		expectSuccess(t, err)
		expectObjectInfos(t, ois)
		ois, err = iter.NextPageDetailed(3)
		expectSuccess(t, err)
		expectObjectInfos(t, ois)

		//test Foreach
		c.Invalidate()
		iter = c.Objects()
		iter.Prefix = "schwift-test-listing"
		idx := 0
		expectSuccess(t, iter.Foreach(func(o *schwift.Object) error {
			idx++
			expectString(t, o.Name(), oname(idx))
			return nil
		}))
		expectInt(t, idx, 4)
		expectContainerHeadersCached(t, c)

		//test ForeachDetailed
		c.Invalidate()
		iter = c.Objects()
		iter.Prefix = "schwift-test-listing"
		idx = 0
		expectSuccess(t, iter.ForeachDetailed(func(info schwift.ObjectInfo) error {
			idx++
			expectString(t, info.Object.Name(), oname(idx))
			return nil
		}))
		expectInt(t, idx, 4)
		expectContainerHeadersCached(t, c)

		//test Collect
		iter = c.Objects()
		iter.Prefix = "schwift-test-listing"
		os, err = iter.Collect()
		expectSuccess(t, err)
		expectObjectNames(t, os, oname(1), oname(2), oname(3), oname(4))

		//test CollectDetailed
		iter = c.Objects()
		iter.Prefix = "schwift-test-listing"
		ois, err = iter.CollectDetailed()
		expectSuccess(t, err)
		expectObjectInfos(t, ois, oname(1), oname(2), oname(3), oname(4))
	})
}

func TestPseudoDirectories(t *testing.T) {
	testWithContainer(t, func(c *schwift.Container) {
		//create test objects that can be listed
		objectNames := []string{
			"foo/1",
			"foo/2",
			"foo/3",
			"foo/bar",
			"foo/bar/1",
			"foo/bar/2",
			"foo/bar/3",
		}
		for _, name := range objectNames {
			hdr := schwift.NewObjectHeaders()
			hdr.ContentType().Set("application/json")
			err := c.Object(name).Upload(bytes.NewReader(objectExampleContent), nil, hdr.ToOpts())
			expectSuccess(t, err)
		}

		//test iteration with Delimiter and no Prefix
		iter := c.Objects()
		iter.Delimiter = "/"
		os, err := iter.Collect()
		expectSuccess(t, err)
		expectObjectNames(t, os, "foo/")

		iter = c.Objects()
		iter.Delimiter = "/"
		ois, err := iter.CollectDetailed()
		expectSuccess(t, err)
		expectObjectInfos(t, ois, "subdir:foo/")

		//test iteration with Delimited and Prefix
		iter = c.Objects()
		iter.Prefix = "foo/"
		iter.Delimiter = "/"
		os, err = iter.Collect()
		expectSuccess(t, err)
		expectObjectNames(t, os, "foo/1", "foo/2", "foo/3", "foo/bar", "foo/bar/")

		iter = c.Objects()
		iter.Prefix = "foo/"
		iter.Delimiter = "/"
		ois, err = iter.CollectDetailed()
		expectSuccess(t, err)
		expectObjectInfos(t, ois, "foo/1", "foo/2", "foo/3", "foo/bar", "subdir:foo/bar/")
	})
}

func TestObjectIteratorWithSymlinks(t *testing.T) {
	testWithContainer(t, func(c *schwift.Container) {
		//create test objects that can be listed
		objectNames := []string{
			"foo/1",
			"foo/3",
		}
		for _, name := range objectNames {
			hdr := schwift.NewObjectHeaders()
			hdr.ContentType().Set("application/json")
			err := c.Object(name).Upload(bytes.NewReader(objectExampleContent), nil, hdr.ToOpts())
			expectSuccess(t, err)
		}

		//create a test symlink
		expectSuccess(t, c.Object("foo/2").SymlinkTo(c.Object("foo/1"), nil, nil))

		iter := c.Objects()
		os, err := iter.Collect()
		expectSuccess(t, err)
		expectObjectNames(t, os, "foo/1", "foo/2", "foo/3")

		iter = c.Objects()
		ois, err := iter.CollectDetailed()
		expectSuccess(t, err)
		expectObjectInfos(t, ois, "foo/1", "symlink:foo/2>foo/1", "foo/3")
	})
}

func expectContainerHeadersCached(t *testing.T, c *schwift.Container) {
	requestCountBefore := c.Account().Backend().(*RequestCountingBackend).Count
	_, err := c.Headers()
	expectSuccess(t, err)
	requestCountAfter := c.Account().Backend().(*RequestCountingBackend).Count

	t.Helper()
	if requestCountBefore != requestCountAfter {
		t.Error("Container.Headers() was expected to use cache, but issued HEAD request")
	}
}

func expectObjectNames(t *testing.T, actualObjects []*schwift.Object, expectedNames ...string) {
	t.Helper()
	if len(actualObjects) != len(expectedNames) {
		t.Errorf("expected %d objects, got %d objects",
			len(expectedNames), len(actualObjects))
		return
	}
	for idx, c := range actualObjects {
		if c.Name() != expectedNames[idx] {
			t.Errorf("expected objects[%d].Name() == %q, got %q",
				idx, expectedNames[idx], c.Name())
		}
	}
}

func expectObjectInfos(t *testing.T, actualInfos []schwift.ObjectInfo, expectedNames ...string) {
	t.Helper()
	if len(actualInfos) != len(expectedNames) {
		t.Errorf("expected %d objects, got %d objects",
			len(expectedNames), len(actualInfos))
		return
	}
	for idx, info := range actualInfos {
		//case 1: pseudo-directory
		if strings.HasPrefix(expectedNames[idx], "subdir:") {
			expectedSubdir := strings.TrimPrefix(expectedNames[idx], "subdir:")
			if expectedSubdir != info.SubDirectory {
				t.Errorf("expected objects[%d] subdir = %q, got %q",
					idx, expectedSubdir, info.SubDirectory)
			}
			if info != (schwift.ObjectInfo{SubDirectory: info.SubDirectory}) {
				t.Errorf("expected objects[%d] to be a subdir, got %#v",
					idx, info)
			}
			continue
		}
		if info.SubDirectory != "" {
			t.Errorf("expected objects[%d] to be an object, got subdir = %q",
				idx, info.SubDirectory)
		}

		//case 2: symlink
		if strings.HasPrefix(expectedNames[idx], "symlink:") {
			fields := strings.SplitN(strings.TrimPrefix(expectedNames[idx], "symlink:"), ">", 2)
			expectedName, expectedTargetName := fields[0], fields[1]

			if info.Object == nil {
				t.Errorf("expected objects[%d].Name() == %q, got object == nil",
					idx, expectedNames[idx])
			} else if info.Object.Name() != expectedName {
				t.Errorf("expected objects[%d].Name() == %q, got %q",
					idx, expectedName, info.Object.Name())
			}

			if info.SymlinkTarget == nil {
				t.Errorf("expected objects[%d] symlinkTarget.Name() == %q, got symlinkTarget == nil",
					idx, expectedTargetName)
			} else if info.SymlinkTarget.Name() != expectedTargetName {
				t.Errorf("expected objects[%d] symlinkTarget.Name() == %q, got %q",
					idx, expectedTargetName, info.SymlinkTarget.Name())
			}

			if info.SizeBytes != 0 {
				t.Errorf("expected objects[%d] sizeBytes == 0, got %d",
					idx, info.SizeBytes)
			}
			if info.ContentType != "application/symlink" {
				t.Errorf(`expected objects[%d] contentType == "application/symlink", got %q`,
					idx, info.ContentType)
			}
			emptyEtag := etagOf(nil)
			if info.Etag != emptyEtag {
				t.Errorf("expected objects[%d] etag == %q, got %q",
					idx, emptyEtag, info.Etag)
			}
			if info.LastModified.IsZero() {
				t.Errorf("objects[%d].LastModified is zero", idx)
			}
			continue
		}

		//case 3: regular object
		if info.Object == nil {
			t.Errorf("expected objects[%d].Name() == %q, got object == nil",
				idx, expectedNames[idx])
		} else if info.Object.Name() != expectedNames[idx] {
			t.Errorf("expected objects[%d].Name() == %q, got %q",
				idx, expectedNames[idx], info.Object.Name())
		}
		if info.SizeBytes != uint64(len(objectExampleContent)) {
			t.Errorf("expected objects[%d] sizeBytes == %d, got %d",
				idx, len(objectExampleContent), info.SizeBytes)
		}
		if info.ContentType != "application/json" {
			t.Errorf(`expected objects[%d] contentType == "application/json", got %q`,
				idx, info.ContentType)
		}
		if info.Etag != objectExampleContentEtag {
			t.Errorf("expected objects[%d] etag == %q, got %q",
				idx, objectExampleContentEtag, info.Etag)
		}
		if info.LastModified.IsZero() {
			t.Errorf("objects[%d].LastModified is zero", idx)
		}
	}
}
