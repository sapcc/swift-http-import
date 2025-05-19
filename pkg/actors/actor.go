// SPDX-FileCopyrightText: 2017 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package actors

import (
	"context"
	"sync"
)

// Actor is something that can be run in its own goroutine.
// This package contains various structs that satisfy this interface,
// and which make up the bulk of the behavior of swift-http-import.
type Actor interface {
	Run(ctx context.Context)
}

// Start runs the given Actor in its own goroutine.
func Start(ctx context.Context, a Actor, wgs ...*sync.WaitGroup) {
	for _, wg := range wgs {
		wg.Add(1)
	}
	go func() {
		defer func() {
			for _, wg := range wgs {
				wg.Done()
			}
		}()
		a.Run(ctx)
	}()
}
