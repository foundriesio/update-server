// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package templates

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func TestJSONEscapesScriptEndTag(t *testing.T) {
	tmpl, err := Templates.Clone()
	if err != nil {
		t.Fatal(err)
	}
	tmpl, err = tmpl.New("json-xss").Parse(`<script>const data = {{json .}};</script>`)
	if err != nil {
		t.Fatal(err)
	}

	var got bytes.Buffer
	data := map[string]string{"value": `</script><script>alert("xss")</script>`}
	if err := tmpl.ExecuteTemplate(&got, "json-xss", data); err != nil {
		t.Fatal(err)
	}
	if strings.Count(got.String(), "</script>") != 1 {
		t.Fatalf("rendered template contains an injected script end tag: %s", got.String())
	}
	if !strings.Contains(got.String(), `\u003c/script\u003e`) {
		t.Fatalf("rendered template does not escape the script end tag: %s", got.String())
	}
}

func TestOtherLabels(t *testing.T) {
	got := OtherLabels(map[string]string{
		"name":  "device-alpha",
		"group": "alpha",
		"env":   "production",
		"site":  "dallas",
	})
	want := []LabelPair{
		{Key: "env", Value: "production"},
		{Key: "site", Value: "dallas"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("OtherLabels() = %#v, want %#v", got, want)
	}
}

func TestOtherLabels_OnlyNameAndGroup(t *testing.T) {
	got := OtherLabels(map[string]string{"name": "device-alpha", "group": "alpha"})
	if len(got) != 0 {
		t.Fatalf("OtherLabels() = %#v, want empty slice", got)
	}
}

func TestOtherLabels_Nil(t *testing.T) {
	got := OtherLabels(nil)
	if len(got) != 0 {
		t.Fatalf("OtherLabels(nil) = %#v, want empty slice", got)
	}
}

func TestInitials(t *testing.T) {
	got := Initials("jdoe")
	want := "J"
	if got != want {
		t.Errorf("Initials(%q) = %q, want %q", "jdoe", got, want)
	}
}

func TestInitials_Empty(t *testing.T) {
	got := Initials("")
	want := ""
	if got != want {
		t.Errorf("Initials(%q) = %q, want %q", "", got, want)
	}
}

func TestInitials_UnicodeFirstChar(t *testing.T) {
	got := Initials("åsa")
	want := "Å"
	if got != want {
		t.Errorf("Initials(%q) = %q, want %q", "åsa", got, want)
	}
}
