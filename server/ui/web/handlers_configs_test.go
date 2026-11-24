// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package web

import (
	"bytes"
	"strings"
	"testing"

	"github.com/foundriesio/update-server/server/ui/api"
	"github.com/foundriesio/update-server/server/ui/i18n"
	"github.com/foundriesio/update-server/server/ui/web/templates"
	"github.com/foundriesio/update-server/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/language"
)

func TestConfigFileEditorUsesSharedDialogContract(t *testing.T) {
	ctx := struct {
		baseCtx
		Configs api.ConfigFileSet
		Groups  []string
		CanEdit bool
		PubKey  string
	}{
		baseCtx: baseCtx{L: i18n.For(language.English)},
		Configs: api.ConfigFileSet{Files: map[string]api.ConfigFile{}},
		CanEdit: true,
	}

	var rendered bytes.Buffer
	require.NoError(t, templates.Templates.ExecuteTemplate(&rendered, "configs_list.html", ctx))
	html := rendered.String()

	for _, expected := range []string{
		`aria-labelledby="config-file-dialog-title"`,
		`class="upload-dialog config-file-dialog"`,
		`class="dlg-header"`,
		`class="dlg-close"`,
		`class="dlg-body"`,
		`class="dlg-footer"`,
		`class="dlg-btn`,
		`for="config-file-name"`,
		`id="config-file-name" name="FileName" required`,
		`for="config-file-reason"`,
		`id="config-file-reason"`,
		`for="config-file-on-changed"`,
		`id="config-file-on-changed"`,
		`for="config-file-encrypted"`,
		`id="config-file-encrypted"`,
		`<label class="config-file-checkbox" for="config-file-encrypted">`,
		`for="config-file-value"`,
		`id="config-file-value"`,
		`aria-label="Close dialog"`,
		`<form autocomplete="off">`,
		`class="config-file-dialog__status" data-config-file-status aria-live="polite" role="status" tabindex="-1"`,
		`<code translate="no">/usr/share/fioconfig/handlers/</code>`,
		`<span data-mode="add">Add Config File</span>`,
		`<span data-mode="edit">Save Changes</span>`,
		`<svg`,
		`aria-hidden="true"`,
		`let saveInFlight = false`,
		`dialogForm.reportValidity()`,
		`setAttribute('aria-busy'`,
		`[data-config-file-status]`,
		`dialog.addEventListener('cancel'`,
		"dialog.addEventListener('close', () => {\n          if (saveInFlight && !dialog.open) {\n            dialog.showModal();\n          }",
		"deleteDialog.addEventListener('close', () => {\n            if (deleteInFlight && !deleteDialog.open) {\n              deleteDialog.showModal();\n            }",
		`id="DeleteConfigFile"`,
		`aria-labelledby="config-file-delete-title"`,
		`id="config-file-delete-form"`,
		`data-config-file-delete-name`,
		`for="config-file-delete-reason"`,
		`id="config-file-delete-reason" name="Reason"`,
		`data-config-file-delete-status`,
		`class="dlg-btn dlg-btn-delete"`,
		`deleteDialog.showModal()`,
		`deleteForm.addEventListener('submit'`,
		`deleteForm.setAttribute('aria-busy'`,
		`id="OverwriteConfigFile"`,
		`aria-labelledby="config-file-overwrite-title"`,
		`data-config-file-overwrite-name`,
		`overwriteFileName.textContent = dialog.dataset.overwriteFormat`,
		`id="config-file-overwrite-cancel-btn"`,
		`id="config-file-overwrite-confirm-btn"`,
		`overwriteDialog.showModal()`,
		`overwriteConfirmButton.addEventListener('click'`,
		`overwriteCancelButton.addEventListener('click'`,
		`overwriteDialog.addEventListener('close'`,
		`setStatus('');
              overwriteFileName.textContent = dialog.dataset.overwriteFormat`,
		`if (!saveInFlight) {
            dialogFileName.focus();
          }`,
	} {
		assert.Contains(t, html, expected)
	}
	assert.NotContains(t, html, "<style>", "dialog presentation belongs in style.css")
	assert.NotContains(t, html, `>x</button>`)
	assert.NotContains(t, html, `alert('Error saving config file:`)
	assert.NotContains(t, html, `window.prompt(`)
	assert.NotContains(t, html, `confirm(`)
	assert.NotContains(t, html, `alert('Error deleting config file:`)
}

func TestConfigFileActionMatchesEditPermission(t *testing.T) {
	configs := api.ConfigFileSet{Files: map[string]api.ConfigFile{
		"example.conf": {
			Value:       "setting=true",
			Unencrypted: true,
			OnChanged:   []string{"reload-example"},
		},
	}}

	for _, tt := range []struct {
		name        string
		canEdit     bool
		want        []string
		doesNotWant []string
	}{
		{
			name:    "editable files open the editor directly",
			canEdit: true,
			want: []string{
				`name="EditFile"`,
				`aria-label="Edit file content"`,
				`<i class="edit" aria-hidden="true"></i>`,
				`<template class="config-file-edit-source">`,
			},
			doesNotWant: []string{
				`aria-label="View or edit file content"`,
				`<i class="gear"></i>`,
				`style="transform: translateY(0.1em)"`,
				`<dialog class="file-content">`,
			},
		},
		{
			name:    "read-only files retain the viewer",
			canEdit: false,
			want: []string{
				`aria-label="View file content"`,
				`<dialog class="file-content">`,
			},
			doesNotWant: []string{
				`name="EditFile"`,
				`<template class="config-file-edit-source">`,
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := struct {
				baseCtx
				Configs api.ConfigFileSet
				Groups  []string
				CanEdit bool
				PubKey  string
			}{baseCtx: baseCtx{L: i18n.For(language.English)}, Configs: configs, CanEdit: tt.canEdit}

			var rendered bytes.Buffer
			require.NoError(t, templates.Templates.ExecuteTemplate(&rendered, "configs_list.html", ctx))
			html := rendered.String()
			for _, expected := range tt.want {
				assert.Contains(t, html, expected)
			}
			for _, unexpected := range tt.doesNotWant {
				assert.NotContains(t, html, unexpected)
			}
		})
	}
}

func TestConfigFileEditorRendersInSharedContexts(t *testing.T) {
	configs := api.ConfigFileSet{Files: map[string]api.ConfigFile{
		"long.conf": {
			Value:       strings.Repeat("value ", 200),
			Unencrypted: true,
			OnChanged:   []string{strings.Repeat("handler/", 40)},
		},
	}}

	tests := []struct {
		name     string
		template string
		ctx      any
	}{
		{
			name:     "global without encryption",
			template: "configs_list.html",
			ctx: struct {
				baseCtx
				Configs api.ConfigFileSet
				Groups  []string
				CanEdit bool
				PubKey  string
			}{baseCtx: baseCtx{L: i18n.For(language.English)}, Configs: configs, CanEdit: true},
		},
		{
			name:     "group without encryption",
			template: "configs_item.html",
			ctx: struct {
				baseCtx
				Group   string
				Configs api.ConfigFileSet
				History []historyRevision
				CanEdit bool
				PubKey  string
			}{baseCtx: baseCtx{L: i18n.For(language.English)}, Group: "factory", Configs: configs, CanEdit: true},
		},
		{
			name:     "device with encryption",
			template: "configs_device_item.html",
			ctx: struct {
				baseCtx
				Device  api.Device
				Configs api.ConfigFileSet
				History []historyRevision
				CanEdit bool
				PubKey  string
			}{baseCtx: baseCtx{L: i18n.For(language.English)}, Configs: configs, CanEdit: true, PubKey: "device-public-key"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var rendered bytes.Buffer
			require.NoError(t, templates.Templates.ExecuteTemplate(&rendered, tt.template, tt.ctx))
			html := rendered.String()
			assert.Contains(t, html, `id="EditConfigsFile"`)
			for _, name := range []string{"FileName", "Reason", "OnChanged", "Encrypted", "Value"} {
				assert.Contains(t, html, `name="`+name+`"`)
			}
			if strings.Contains(tt.name, "with encryption") {
				assert.NotContains(t, html, `data-mode="add" hidden`)
			} else {
				assert.Contains(t, html, `data-mode="add" hidden`)
			}
		})
	}
}

func configsFileSet() api.ConfigFileSet {
	return api.ConfigFileSet{
		Reason:    "initial",
		CreatedBy: "admin",
		Files: map[string]storage.ConfigFile{
			"app.conf": {Value: "a=1", Unencrypted: true, OnChanged: []string{"restart"}},
		},
	}
}

func renderConfigsItemHTML(t *testing.T, tag language.Tag) string {
	t.Helper()

	set := configsFileSet()
	ctx := struct {
		baseCtx
		Group   string
		Configs api.ConfigFileSet
		History []historyRevision
		CanEdit bool
		PubKey  string
	}{
		baseCtx: baseCtx{L: i18n.For(tag), BrandName: "Update Server"},
		Group:   "g1",
		Configs: set,
		History: buildHistoryRevisions([]api.ConfigFileSet{set}),
		CanEdit: true,
	}
	var buf bytes.Buffer
	require.NoError(t, templates.Templates.ExecuteTemplate(&buf, "configs_item.html", ctx))

	return buf.String()
}

func TestConfigsItemRendersEnglish(t *testing.T) {
	got := renderConfigsItemHTML(t, language.English)

	assert.Contains(t, got, "Current Configuration")
	assert.Contains(t, got, "Revision History (1)")
}

func TestConfigsItemRendersItalian(t *testing.T) {
	got := renderConfigsItemHTML(t, language.Italian)

	assert.Contains(t, got, "Configurazione corrente")
	assert.Contains(t, got, "Cronologia delle revisioni (1)")
	// JS strings travel in data attributes, with a {name} slot left for the script.
	assert.Contains(t, got, `data-deleting-message="Eliminazione del file di configurazione…"`)
	assert.Contains(t, got, `data-save-failed-message="Salvataggio del file di configurazione non riuscito."`)
	assert.Contains(t, got, `data-overwrite-format="Esiste già un file con nome &#34;{name}&#34;.`)
	assert.NotContains(t, got, "Current Configuration")
}

func TestConfigsHistoryRendersItalianThroughMap(t *testing.T) {
	ctx := struct {
		baseCtx
		History   []api.ConfigFileSet
		OpenIndex int
	}{
		baseCtx:   baseCtx{L: i18n.For(language.Italian), BrandName: "Update Server"},
		History:   []api.ConfigFileSet{configsFileSet()},
		OpenIndex: 0,
	}
	var buf bytes.Buffer
	require.NoError(t, templates.Templates.ExecuteTemplate(&buf, "configs_history.html", ctx))

	assert.Contains(t, buf.String(), "Motivo")
	assert.NotContains(t, buf.String(), `class="meta-label">Reason<`)
}
