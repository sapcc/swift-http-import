/*******************************************************************************
*
* Copyright 2018 SAP SE
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
