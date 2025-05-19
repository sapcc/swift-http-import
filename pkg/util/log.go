// SPDX-FileCopyrightText: 2016-2017 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package util

import "github.com/sapcc/go-bits/osext"

// LogIndividualTransfers is set to the boolean value of the
// LOG_TRANSFERS environment variable.
var LogIndividualTransfers = osext.GetenvBool("LOG_TRANSFERS")
