// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package templates

import (
	"embed"
	"html/template"
	"strings"
	"time"
)

//go:embed *.html *.css
var Assets embed.FS
var Templates *template.Template

func init() {
	funcMap := template.FuncMap{
		"tsToString": func(ts int64) string {
			return time.Unix(ts, 0).Format(time.RFC3339)
		},
		"add": func(a, b int) int {
			return a + b
		},
		"sub": func(a, b int) int {
			return a - b
		},
		"contains": strings.Contains,
	}

	Templates = template.Must(template.New("").Funcs(funcMap).ParseFS(Assets, "*.html", "*.css"))
}
