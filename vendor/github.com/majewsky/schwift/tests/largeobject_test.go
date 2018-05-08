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

func foreachLargeObjectStrategy(action func(schwift.LargeObjectStrategy, string)) {
	action(schwift.StaticLargeObject, "slo")
	action(schwift.DynamicLargeObject, "dlo")
}

func TestLargeObjectsBasic(t *testing.T) {
	testWithContainer(t, func(c *schwift.Container) {
		foreachLargeObjectStrategy(func(strategy schwift.LargeObjectStrategy, strategyStr string) {

			obj := c.Object(strategyStr + "-largeobject")
			lo, err := obj.AsLargeObject()
			expectError(t, err, schwift.ErrNotLarge.Error())

			segment1 := getRandomSegmentContent(128)
			segment2 := getRandomSegmentContent(128)
			segment3 := getRandomSegmentContent(128)
			segment4 := getRandomSegmentContent(128)

			//basic write example
			lo, err = obj.AsNewLargeObject(schwift.SegmentingOptions{
				SegmentContainer: c,
				SegmentPrefix:    strategyStr + "-segments/",
				Strategy:         strategy,
			}, nil)
			expectSuccess(t, err)
			expectSuccess(t, lo.Append(bytes.NewReader([]byte(segment1+segment2)), 128))
			expectSuccess(t, lo.WriteManifest(nil))

			expectObjectContent(t, obj, []byte(segment1+segment2))
			expectLargeObject(t, obj, []schwift.SegmentInfo{
				{
					Object:    c.Object(strategyStr + "-segments/0000000000000001"),
					SizeBytes: 128,
					Etag:      etagOfString(segment1),
				},
				{
					Object:    c.Object(strategyStr + "-segments/0000000000000002"),
					SizeBytes: 128,
					Etag:      etagOfString(segment2),
				},
			})

			//basic append example
			lo, err = obj.AsLargeObject()
			expectSuccess(t, err)
			expectLargeObjectSetup(t, lo, strategy,
				fmt.Sprintf("%s/%s-segments/", c.Name(), strategyStr))
			expectSuccess(t, lo.Append(bytes.NewReader([]byte(segment3+segment4)), 128))
			expectSuccess(t, lo.WriteManifest(nil))

			expectObjectContent(t, obj, []byte(segment1+segment2+segment3+segment4))
			expectLargeObject(t, obj, []schwift.SegmentInfo{
				{
					Object:    c.Object(strategyStr + "-segments/0000000000000001"),
					SizeBytes: 128,
					Etag:      etagOfString(segment1),
				},
				{
					Object:    c.Object(strategyStr + "-segments/0000000000000002"),
					SizeBytes: 128,
					Etag:      etagOfString(segment2),
				},
				{
					Object:    c.Object(strategyStr + "-segments/0000000000000003"),
					SizeBytes: 128,
					Etag:      etagOfString(segment3),
				},
				{
					Object:    c.Object(strategyStr + "-segments/0000000000000004"),
					SizeBytes: 128,
					Etag:      etagOfString(segment4),
				},
			})

			//basic truncate example
			lo, err = obj.AsLargeObject()
			expectSuccess(t, err)
			err = lo.Truncate(&schwift.TruncateOptions{
				DeleteSegments: true,
			})
			expectSuccess(t, err)
			expectLargeObjectSetup(t, lo, strategy,
				fmt.Sprintf("%s/%s-segments/", c.Name(), strategyStr))

			//verify that segments were deleted
			iter := c.Objects()
			iter.Prefix = lo.SegmentPrefix()
			names, err := iter.Collect()
			expectSuccess(t, err)
			expectObjectNames(t, names)

			expectSuccess(t, lo.Append(bytes.NewReader([]byte(segment3+segment4)), 128))
			expectSuccess(t, lo.WriteManifest(nil))

			expectObjectContent(t, obj, []byte(segment3+segment4))
			expectLargeObject(t, obj, []schwift.SegmentInfo{
				{
					Object:    c.Object(strategyStr + "-segments/0000000000000001"),
					SizeBytes: 128,
					Etag:      etagOfString(segment3),
				},
				{
					Object:    c.Object(strategyStr + "-segments/0000000000000002"),
					SizeBytes: 128,
					Etag:      etagOfString(segment4),
				},
			})

		})
	})
}

func TestTruncateDuringOverwrite(t *testing.T) {
	foreachLargeObjectStrategy(func(strategy schwift.LargeObjectStrategy, strategyStr string) {
		testWithContainer(t, func(c *schwift.Container) {
			obj := c.Object("largeobject")

			//setup phase: create a large object
			lo, err := obj.AsNewLargeObject(schwift.SegmentingOptions{
				SegmentContainer: c,
				SegmentPrefix:    "segments/",
				Strategy:         strategy,
			}, nil)
			expectSuccess(t, err)

			segment1 := getRandomSegmentContent(128)
			segment2 := getRandomSegmentContent(128)
			expectSuccess(t, lo.Append(bytes.NewReader([]byte(segment1)), 0))
			expectSuccess(t, lo.Append(bytes.NewReader([]byte(segment2)), 0))
			expectSuccess(t, lo.WriteManifest(nil))

			expectObjectExistence(t, c.Object("segments/0000000000000001"), true)
			expectObjectExistence(t, c.Object("segments/0000000000000002"), true)

			//test phase: truncate using AsNewLargeObject
			lo, err = obj.AsNewLargeObject(schwift.SegmentingOptions{
				SegmentContainer: c,
				Strategy:         strategy,
			}, &schwift.TruncateOptions{
				DeleteSegments: true,
			})
			expectSuccess(t, err)
			expectSuccess(t, lo.Append(bytes.NewReader([]byte(segment1)), 0))
			expectSuccess(t, lo.Append(bytes.NewReader([]byte(segment2)), 0))
			expectSuccess(t, lo.WriteManifest(nil))

			expectObjectExistence(t, c.Object("segments/0000000000000001"), false)
			expectObjectExistence(t, c.Object("segments/0000000000000002"), false)

		})
	})
}

func TestOpenRegularObjectAsLargeObject(t *testing.T) {
	testWithContainer(t, func(c *schwift.Container) {
		o := c.Object("foo")
		expectSuccess(t, o.Upload(bytes.NewReader(objectExampleContent), nil, nil))
		_, err := o.AsLargeObject()
		expectError(t, err, schwift.ErrNotLarge.Error())
	})
}

func TestSLOWithDataSegment(t *testing.T) {
	testWithContainer(t, func(c *schwift.Container) {
		o := c.Object("foo")
		lo, err := o.AsNewLargeObject(schwift.SegmentingOptions{
			SegmentContainer: c,
			SegmentPrefix:    "segments/",
			Strategy:         schwift.StaticLargeObject,
		}, nil)
		expectSuccess(t, err)

		segment1 := getRandomSegmentContent(128)
		dataSegment := schwift.SegmentInfo{Data: []byte("---")}
		segment2 := getRandomSegmentContent(128)

		expectSuccess(t, lo.Append(bytes.NewReader([]byte(segment1)), 0))
		expectSuccess(t, lo.AddSegment(dataSegment))
		expectSuccess(t, lo.Append(bytes.NewReader([]byte(segment2)), 0))
		expectSuccess(t, lo.WriteManifest(nil))

		expectObjectContent(t, o, []byte(segment1+string(dataSegment.Data)+segment2))
		expectLargeObject(t, o, []schwift.SegmentInfo{
			{
				Object:    c.Object("segments/0000000000000001"),
				SizeBytes: 128,
				Etag:      etagOfString(segment1),
			},
			dataSegment,
			{
				Object:    c.Object("segments/0000000000000002"),
				SizeBytes: 128,
				Etag:      etagOfString(segment2),
			},
		})

		//check that truncating this does not try to delete the nil segment.Object
		//in the data segment
		expectSuccess(t, lo.Truncate(&schwift.TruncateOptions{
			DeleteSegments: true,
		}))
	})
}

func TestSLOWithRangeSegments(t *testing.T) {
	testWithContainer(t, func(c *schwift.Container) {
		segmentStr := "<aaa>X<bbb>X<ccc>"
		segmentObj := c.Object("segment")
		expectSuccess(t, segmentObj.Upload(bytes.NewReader([]byte(segmentStr)), nil, nil))

		o := c.Object("largeobject")
		lo, err := o.AsNewLargeObject(schwift.SegmentingOptions{
			SegmentContainer: c,
			SegmentPrefix:    "segments/",
			Strategy:         schwift.StaticLargeObject,
		}, nil)
		expectSuccess(t, err)

		//the large object is composed out of three ranges such that the "X" are precisely cut out of segmentStr
		expectSuccess(t, lo.AddSegment(schwift.SegmentInfo{
			Object:      segmentObj,
			RangeLength: 5,
		}))
		expectSuccess(t, lo.AddSegment(schwift.SegmentInfo{
			Object:      segmentObj,
			RangeOffset: 6,
			RangeLength: 5,
		}))
		expectSuccess(t, lo.AddSegment(schwift.SegmentInfo{
			Object:      segmentObj,
			RangeOffset: -1,
			RangeLength: 5,
		}))
		expectSuccess(t, lo.WriteManifest(nil))

		expectObjectContent(t, o, []byte(
			strings.Replace(segmentStr, "X", "", -1),
		))
		expectLargeObject(t, o, []schwift.SegmentInfo{
			{
				Object:      segmentObj,
				Etag:        etagOfString(segmentStr),
				SizeBytes:   uint64(len(segmentStr)),
				RangeLength: 5,
			},
			{
				Object:      segmentObj,
				Etag:        etagOfString(segmentStr),
				SizeBytes:   uint64(len(segmentStr)),
				RangeOffset: 6,
				RangeLength: 5,
			},
			{
				Object:      segmentObj,
				Etag:        etagOfString(segmentStr),
				SizeBytes:   uint64(len(segmentStr)),
				RangeOffset: -1,
				RangeLength: 5,
			},
		})
	})
}

func TestSLOGuessSegmentPrefix(t *testing.T) {
	testWithContainer(t, func(c *schwift.Container) {
		obj := c.Object("largeobject")

		//setup phase: create an SLO
		lo, err := obj.AsNewLargeObject(schwift.SegmentingOptions{
			SegmentContainer: c,
			SegmentPrefix:    "foo/bar/baz/",
		}, nil)
		expectSuccess(t, err)

		segment1 := getRandomSegmentContent(128)
		segment2 := getRandomSegmentContent(128)
		expectSuccess(t, lo.Append(bytes.NewReader([]byte(segment1)), 0))
		expectSuccess(t, lo.Append(bytes.NewReader([]byte(segment2)), 0))
		expectSuccess(t, lo.WriteManifest(nil))

		//now create a fresh SLO and check if it infers the correct SegmentPrefix
		lo, err = obj.AsLargeObject()
		expectSuccess(t, err)
		expectString(t, lo.SegmentContainer().Name(), c.Name())
		expectString(t, lo.SegmentPrefix(), "foo/bar/baz/")
	})
}

func TestDeleteLargeObjectAndKeepSegments(t *testing.T) {
	foreachLargeObjectStrategy(func(strategy schwift.LargeObjectStrategy, strategyStr string) {
		testWithContainer(t, func(c *schwift.Container) {
			obj := c.Object("largeobject")

			//setup phase: create a large object
			lo, err := obj.AsNewLargeObject(schwift.SegmentingOptions{
				SegmentContainer: c,
				SegmentPrefix:    "foo/bar/baz/",
				Strategy:         strategy,
			}, nil)
			expectSuccess(t, err)

			segment1 := getRandomSegmentContent(128)
			segment2 := getRandomSegmentContent(128)
			expectSuccess(t, lo.Append(bytes.NewReader([]byte(segment1)), 0))
			expectSuccess(t, lo.Append(bytes.NewReader([]byte(segment2)), 0))
			expectSuccess(t, lo.WriteManifest(nil))

			//test deletion that keeps segments
			expectSuccess(t, obj.Delete(nil, nil))

			iter := c.Objects()
			iter.Prefix = lo.SegmentPrefix()
			names, err := iter.Collect()
			expectSuccess(t, err)
			expectObjectNames(t, names,
				"foo/bar/baz/0000000000000001",
				"foo/bar/baz/0000000000000002")
		})
	})
}

func TestDeleteLargeObjectIncludingSegments(t *testing.T) {
	foreachLargeObjectStrategy(func(strategy schwift.LargeObjectStrategy, strategyStr string) {
		testWithContainer(t, func(c *schwift.Container) {
			obj := c.Object("largeobject")

			//setup phase: create a large object
			lo, err := obj.AsNewLargeObject(schwift.SegmentingOptions{
				SegmentContainer: c,
				SegmentPrefix:    "foo/bar/baz/",
				Strategy:         strategy,
			}, nil)
			expectSuccess(t, err)

			segment1 := getRandomSegmentContent(128)
			segment2 := getRandomSegmentContent(128)
			expectSuccess(t, lo.Append(bytes.NewReader([]byte(segment1)), 0))
			expectSuccess(t, lo.Append(bytes.NewReader([]byte(segment2)), 0))
			expectSuccess(t, lo.WriteManifest(nil))

			//test deletion that keeps segments
			expectSuccess(t, obj.Delete(&schwift.DeleteOptions{DeleteSegments: true}, nil))

			iter := c.Objects()
			iter.Prefix = lo.SegmentPrefix()
			names, err := iter.Collect()
			expectSuccess(t, err)
			expectObjectNames(t, names)
		})
	})
}

func TestOverwriteLargeObjectAndKeepSegments(t *testing.T) {
	foreachLargeObjectStrategy(func(strategy schwift.LargeObjectStrategy, strategyStr string) {
		testWithContainer(t, func(c *schwift.Container) {
			obj := c.Object("largeobject")

			//setup phase: create a large object
			lo, err := obj.AsNewLargeObject(schwift.SegmentingOptions{
				SegmentContainer: c,
				SegmentPrefix:    "foo/bar/baz/",
				Strategy:         strategy,
			}, nil)
			expectSuccess(t, err)

			segment1 := getRandomSegmentContent(128)
			segment2 := getRandomSegmentContent(128)
			expectSuccess(t, lo.Append(bytes.NewReader([]byte(segment1)), 0))
			expectSuccess(t, lo.Append(bytes.NewReader([]byte(segment2)), 0))
			expectSuccess(t, lo.WriteManifest(nil))

			//test overwriting that keeps segments
			expectSuccess(t, obj.Upload(bytes.NewReader(objectExampleContent), nil, nil))

			iter := c.Objects()
			iter.Prefix = lo.SegmentPrefix()
			names, err := iter.Collect()
			expectSuccess(t, err)
			expectObjectNames(t, names,
				"foo/bar/baz/0000000000000001",
				"foo/bar/baz/0000000000000002")
		})
	})
}

func TestOverwriteLargeObjectIncludingSegments(t *testing.T) {
	foreachLargeObjectStrategy(func(strategy schwift.LargeObjectStrategy, strategyStr string) {
		testWithContainer(t, func(c *schwift.Container) {
			obj := c.Object("largeobject")

			//setup phase: create a large object
			lo, err := obj.AsNewLargeObject(schwift.SegmentingOptions{
				SegmentContainer: c,
				SegmentPrefix:    "foo/bar/baz/",
				Strategy:         strategy,
			}, nil)
			expectSuccess(t, err)

			segment1 := getRandomSegmentContent(128)
			segment2 := getRandomSegmentContent(128)
			expectSuccess(t, lo.Append(bytes.NewReader([]byte(segment1)), 0))
			expectSuccess(t, lo.Append(bytes.NewReader([]byte(segment2)), 0))
			expectSuccess(t, lo.WriteManifest(nil))

			//test overwriting that deletes segments
			expectSuccess(t, obj.Upload(
				bytes.NewReader(objectExampleContent),
				&schwift.UploadOptions{DeleteSegments: true},
				nil,
			))

			iter := c.Objects()
			iter.Prefix = lo.SegmentPrefix()
			names, err := iter.Collect()
			expectSuccess(t, err)
			expectObjectNames(t, names)

			//test overwriting that wants to delete segments, but there aren't any
			expectSuccess(t, obj.Upload(
				bytes.NewReader(objectExampleContent),
				&schwift.UploadOptions{DeleteSegments: true},
				nil,
			))

			//while we're at it, test the same for deletion
			expectSuccess(t, obj.Delete(
				&schwift.DeleteOptions{DeleteSegments: true},
				nil,
			))
		})
	})
}

func TestAddInvalidSegments(t *testing.T) {
	foreachLargeObjectStrategy(func(strategy schwift.LargeObjectStrategy, strategyStr string) {
		testWithContainer(t, func(c *schwift.Container) {
			obj := c.Object("largeobject")
			lo, err := obj.AsNewLargeObject(schwift.SegmentingOptions{
				SegmentContainer: c,
				Strategy:         strategy,
			}, nil)
			expectSuccess(t, err)

			//must have either backing object or data
			expectError(t, lo.AddSegment(schwift.SegmentInfo{
				Object: nil,
				Data:   nil,
			}), schwift.ErrSegmentInvalid.Error())

			//if data segment, must not have any other attribute
			expectError(t, lo.AddSegment(schwift.SegmentInfo{
				Data:   []byte("foo"),
				Object: obj,
			}), schwift.ErrSegmentInvalid.Error())
			expectError(t, lo.AddSegment(schwift.SegmentInfo{
				Data:      []byte("foo"),
				SizeBytes: 3,
			}), schwift.ErrSegmentInvalid.Error())
			expectError(t, lo.AddSegment(schwift.SegmentInfo{
				Data: []byte("foo"),
				Etag: etagOfString("foo"),
			}), schwift.ErrSegmentInvalid.Error())
			expectError(t, lo.AddSegment(schwift.SegmentInfo{
				Data:        []byte("foo"),
				RangeOffset: 2,
			}), schwift.ErrSegmentInvalid.Error())
			expectError(t, lo.AddSegment(schwift.SegmentInfo{
				Data:        []byte("foo"),
				RangeLength: 1,
			}), schwift.ErrSegmentInvalid.Error())

			//malformed range
			expectError(t, lo.AddSegment(schwift.SegmentInfo{
				Object:      obj,
				RangeOffset: -1,
				RangeLength: 0,
			}), schwift.ErrSegmentInvalid.Error())

			//DLO is strict about the SegmentContainer and SegmentPrefix, but SLO accepts it
			c2 := c.Account().Container("foo")
			err = lo.AddSegment(schwift.SegmentInfo{
				Object: c2.Object("foo"),
			})
			if strategy == schwift.DynamicLargeObject {
				expectError(t, err, schwift.ErrContainerMismatch.Error())
			} else {
				expectSuccess(t, err)
			}
			err = lo.AddSegment(schwift.SegmentInfo{
				Object: c.Object("definitely-not-in-the-segment-prefix"),
			})
			if strategy == schwift.DynamicLargeObject {
				expectError(t, err, schwift.ErrContainerMismatch.Error())
			} else {
				expectSuccess(t, err)
			}
		})
	})
}

////////////////////////////////////////////////////////////////////////////////
// helpers

func expectLargeObject(t *testing.T, obj *schwift.Object, expected []schwift.SegmentInfo) {
	t.Helper()
	expectObjectExistence(t, obj, true)
	lo, err := obj.AsLargeObject()
	expectSuccess(t, err)
	if lo == nil {
		t.FailNow()
	}

	actual, err := lo.Segments()
	expectSuccess(t, err)
	if len(actual) != len(expected) {
		t.Errorf("expected %s to have %d segments, got %d segments",
			obj.FullName(), len(expected), len(actual))
		return
	}

	for idx, as := range actual {
		es := expected[idx]
		if len(es.Data) > 0 {
			//expecting data segment
			if string(es.Data) != string(as.Data) {
				t.Errorf("expected segments[%d].Data == %q, got %q",
					idx, string(es.Data), string(as.Data))
			}
		} else {
			//expecting segment backed by object
			if as.Object.FullName() != es.Object.FullName() {
				t.Errorf("expected segments[%d].Object.FullName() == %q, got %q",
					idx, es.Object.FullName(), as.Object.FullName())
			}
			if es.SizeBytes != 0 && as.SizeBytes != es.SizeBytes {
				t.Errorf("expected segments[%d].SizeBytes == %d, got %d",
					idx, es.SizeBytes, as.SizeBytes)
			}
			if es.Etag != "" && as.Etag != es.Etag {
				t.Errorf("expected segments[%d].Etag == %q, got %q",
					idx, es.Etag, as.Etag)
			}
		}
	}
}

func expectLargeObjectSetup(t *testing.T, lo *schwift.LargeObject, strategy schwift.LargeObjectStrategy, segmentFullPrefix string) {
	if strategy != lo.Strategy() {
		t.Errorf("expected %s to use LargeObjectStrategy %d, got %d",
			lo.Object().FullName(), strategy, lo.Strategy())
	}

	if lo.SegmentContainer() == nil {
		t.Errorf("expected %s to use segment container+prefix %q, got no container",
			lo.Object().FullName(), segmentFullPrefix)
	} else {
		fullPrefix := lo.SegmentContainer().Name() + "/" + lo.SegmentPrefix()
		if fullPrefix != segmentFullPrefix {
			t.Errorf("expected %s to use segment container+prefix %q, got %q",
				lo.Object().FullName(), segmentFullPrefix, fullPrefix)
		}
	}
}
