// SPDX-FileCopyrightText: 2018 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package objects

import (
	"testing"
	"time"

	"github.com/sapcc/go-bits/assert"
	"github.com/sapcc/go-bits/must"
	yaml "gopkg.in/yaml.v2"
)

func TestAgeSpecDemarshal(t *testing.T) {
	testcases := map[string]AgeSpec{
		"30 seconds": AgeSpec(30 * time.Second),
		"1 minute":   AgeSpec(1 * time.Minute),
		"5 h":        AgeSpec(5 * time.Hour),
		"1s":         AgeSpec(1 * time.Second),
	}

	for input, expected := range testcases {
		t.Run("parse "+input, func(t *testing.T) {
			var actual struct {
				Age AgeSpec `yaml:"age"`
			}
			must.SucceedT(t, yaml.Unmarshal([]byte("age: "+input), &actual))
			assert.Equal(t, actual.Age, expected)
		})
	}
}
