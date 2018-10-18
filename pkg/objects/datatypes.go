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
	"fmt"
	"regexp"
	"strconv"
	"time"
)

//AgeSpec is a timestamp that is deserialized from a duration in the format
//"<value> <unit>", e.g. "4 days" or "2 weeks".
type AgeSpec time.Duration

var ageSpecRx = regexp.MustCompile(`^\s*([0-9]+)\s*(\w+)\s*$`)
var ageSpecUnits = map[string]time.Duration{
	"seconds": time.Second,
	"second":  time.Second,
	"s":       time.Second,

	"minutes": time.Minute,
	"minute":  time.Minute,
	"m":       time.Minute,

	"hours": time.Hour,
	"hour":  time.Hour,
	"h":     time.Hour,

	"days": 24 * time.Hour,
	"day":  24 * time.Hour,
	"d":    24 * time.Hour,

	"weeks": 24 * 7 * time.Hour,
	"week":  24 * 7 * time.Hour,
	"w":     24 * 7 * time.Hour,
}

//UnmarshalYAML implements the yaml.Unmarshaler interface.
func (a *AgeSpec) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var input string
	err := unmarshal(&input)
	if err != nil {
		return err
	}

	match := ageSpecRx.FindStringSubmatch(input)
	if match == nil {
		return fmt.Errorf(`expected age specification in the format "<value> <unit>", e.g. "2h" or "4 days", got %q instead`, input)
	}

	count, err := strconv.ParseUint(match[1], 10, 16)
	if err != nil {
		return err
	}

	unit, ok := ageSpecUnits[match[2]]
	if !ok {
		return fmt.Errorf("unknown unit %q", match[2])
	}

	*a = AgeSpec(unit * time.Duration(count))
	return nil
}
