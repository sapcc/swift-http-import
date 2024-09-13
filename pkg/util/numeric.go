/*******************************************************************************
*
* Copyright 2024 SAP SE
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

package util

// AtLeastZero safely converts int or int64 values (such as return values from
// IO reads/writes) to uint64 by clamping negative values to 0.
func AtLeastZero[I interface{ int | int64 }](x I) uint64 {
	if x < 0 {
		return 0
	}
	return uint64(x)
}

// PointerTo constructs a pointer to a provided value.
func PointerTo[T any](value T) *T {
	return &value
}
