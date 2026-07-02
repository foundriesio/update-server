// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package templates

import (
	"embed"
	"fmt"
	"html/template"
	"strings"
	"time"

	"github.com/foundriesio/update-server/clock"
)

//go:embed *.html *.css
var Assets embed.FS
var Templates *template.Template

func init() {
	funcMap := template.FuncMap{
		"map": func(kv ...any) (map[string]any, error) {
			if len(kv)%2 != 0 {
				return nil, fmt.Errorf("map only accepts an even number of arguments, but got %d", len(kv))
			}
			res := make(map[string]any, len(kv)/2)
			for i := 0; i < len(kv); i += 2 {
				if key, ok := kv[i].(string); !ok {
					return nil, fmt.Errorf("map even arguments must be a string, but got %T for %d", kv[i], i)
				} else {
					res[key] = kv[i+1]
				}
			}
			return res, nil
		},
		"tsToString": func(ts int64) string {
			return time.Unix(ts, 0).Format(time.RFC3339)
		},
		"isExpired": func(expires any) bool {
			s, ok := expires.(string)
			if !ok || s == "" {
				return false
			}
			t, err := time.Parse(time.RFC3339, s)
			if err != nil {
				return false
			}
			return clock.Now().After(t)
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
