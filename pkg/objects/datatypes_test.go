// SPDX-FileCopyrightText: 2018 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package objects

import (
	"testing"
	"time"

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
		var actual struct {
			Age AgeSpec `yaml:"age"`
		}
		err := yaml.Unmarshal([]byte("age: "+input), &actual)
		if err != nil {
			t.Errorf("unexpected unmarshal error for input %q: %s", input, err.Error())
		}
		if actual.Age != expected {
			t.Errorf("expected %q to parse into %d, but got %d", input, expected, actual.Age)
		}
	}
}
