// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

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
