// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package golden

func f(p printer) {
	p.T("Save")
	p.N("%[1]s device", "%[1]s devices", 2)
	p.T("From Go")
}
