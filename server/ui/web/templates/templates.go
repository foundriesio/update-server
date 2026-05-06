// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package templates

import (
	"embed"
	"html/template"
	"reflect"
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
		"deref": func(v any) any {
			// Used to convert e.g. *bool to bool in config_item.html.
			if val := reflect.ValueOf(v); val.Kind() == reflect.Ptr && !val.IsNil() {
				return val.Elem().Interface()
			}
			return v
		},
	}

	Templates = template.Must(template.New("").Funcs(funcMap).ParseFS(Assets, "*.html", "*.css"))
}
