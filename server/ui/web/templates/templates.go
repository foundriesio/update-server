// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package templates

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"slices"
	"strings"
	"time"

	"github.com/foundriesio/update-server/clock"
)

//go:embed *.html *.css favicon.svg
var Assets embed.FS
var Templates *template.Template

// LabelPair is a single device label key/value, used by devices_list.html's
// "+N" label-expand widget (see OtherLabels).
type LabelPair struct {
	Key   string
	Value string
}

// OtherLabels returns labels other than "name"/"group" (which already have
// their own table columns), sorted by key for deterministic rendering.
func OtherLabels(labels map[string]string) []LabelPair {
	out := make([]LabelPair, 0, len(labels))
	for k, v := range labels {
		if k == "name" || k == "group" {
			continue
		}
		out = append(out, LabelPair{Key: k, Value: v})
	}
	slices.SortFunc(out, func(a, b LabelPair) int { return strings.Compare(a.Key, b.Key) })
	return out
}

// Initials returns a single uppercase letter derived from username — the
// first character, uppercased. Usernames here aren't necessarily real names
// (could be an email, "admin", etc.), so this deliberately doesn't attempt
// multi-word initials (e.g. "John Doe" -> "JD").
func Initials(username string) string {
	if username == "" {
		return ""
	}
	r := []rune(username)
	return strings.ToUpper(string(r[0]))
}

func init() {
	// go:embed bakes in whatever bytes the checkout had; a clone without
	// git-lfs leaves pointer stubs and an unstyled UI
	entries, err := Assets.ReadDir(".")
	if err != nil {
		panic(err)
	}
	for _, e := range entries {
		data, err := Assets.ReadFile(e.Name())
		if err != nil {
			panic(err)
		}
		if bytes.HasPrefix(data, []byte("version https://git-lfs.github.com/spec")) {
			panic(fmt.Sprintf("UI asset %s is a Git-LFS pointer stub; run 'git lfs pull' and rebuild", e.Name()))
		}
	}

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
			return time.Unix(ts, 0).Format(time.DateOnly)
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
		"contains":    strings.Contains,
		"otherLabels": OtherLabels,
		"initials":    Initials,
		"json": func(v any) (template.JS, error) {
			b, err := json.Marshal(v)
			return template.JS(b), err
		},
	}

	Templates = template.Must(template.New("").Funcs(funcMap).ParseFS(Assets, "*.html", "*.css"))
}
