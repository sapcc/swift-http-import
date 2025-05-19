// SPDX-FileCopyrightText: 2017 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package objects

// Directory describes a directory on the source side which can be scraped.
type Directory struct {
	Job  *Job
	Path string
	// RetryCounter is increased by the actors.Scraper when scraping of this
	// directory fails.
	RetryCounter uint
}
