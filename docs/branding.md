# Branding the Update Server

You can visually rebrand the server at runtime — no recompilation or rebuild required.
Drop your assets into a directory, edit a JSON file, and restart.

## The branding directory

Branding files live in a `branding/` subdirectory of the server's data directory:

```
<configdir>/branding/
```

The directory is entirely optional. If it is absent, the server uses its built-in
default look with no further configuration needed.

## branding.json

Place a `branding.json` file in `<configdir>/branding/` to control the server's
appearance. Every field is optional; any field you omit, or that carries an invalid
value, falls back silently to its built-in default.

* `title` (string) — Application name shown in the top navigation bar and appended
  to every browser tab title (for example, a title of `Acme Device Updates` makes
  the Devices page read "Devices - Acme Device Updates"). Default: `Foundries Update Server`.

* `logo` (string) — Filename of an image to display in the top navigation bar in
  place of the title text. Any browser-displayable image format is accepted
  (SVG, PNG, JPEG, …). The file must be a plain filename with no path components
  and must exist in `<configdir>/branding/`. If omitted or the file is missing,
  the title text is shown instead.

* `favicon` (string) — Filename of the browser tab and bookmark icon. Accepted
  formats: `.svg`, `.ico`, `.png`. The file must be a plain filename in
  `<configdir>/branding/`. If omitted, or if the filename contains a path
  component or has an unsupported extension, the built-in default favicon is served.

* `colors` (object) — Light-mode theme colors (see also `colorsDark` for dark-mode
  overrides). Each value is a CSS color: hex (`#rrggbb`), `rgb()` / `rgba()`,
  `hsl()`, or a CSS named color. Invalid values are ignored and that color keeps
  its default.

  * `primary` — Top bar background and navigation accents. Default: `rgb(2, 11, 64)`.
  * `accent` — Hover and highlight color; also used as the interactive/link color in
    dark mode. Default: `#a3b4ff`.
  * `surface` — Page background. Default: `#f2f2f2`.
  * `surface-alt` — Content and card background. Default: `#ffffff`.
  * `text` — Main text color. Default: `#000000`.

* `colorsDark` (object, optional) — Dark-mode surface and text overrides. Mirrors the
  `colors` object in format but only three keys are applied: `surface`, `surface-alt`,
  and `text`. The `primary` and `accent` brand colors are shared across both modes —
  placing them inside `colorsDark` is accepted but has no effect; dark-mode interactive
  color is always derived from the `accent` value in `colors`. Each value follows the
  same CSS color formats and fail-soft validation as `colors` (an invalid value leaves
  that dark field at its default). Omitting `colorsDark` entirely, or omitting
  individual keys within it, keeps the built-in dark defaults for those fields.

  * `surface` — Page background in dark mode. Default: `#11191f`.
  * `surface-alt` — Content and card background in dark mode. Default: `#1a2632`.
  * `text` — Main text color in dark mode. Default: `#eef1f4`.

> [!IMPORTANT]
> Branding is read once at startup. If `branding.json` is present but contains
> invalid JSON, the server logs a warning and falls back to all defaults for that
> run. After any change to `branding.json` or its assets, restart the server for
> the changes to take effect.

## Logos and favicons

Asset files must be plain filenames — no subdirectory paths. The server will not
serve a file whose name contains a `/` or `\`. Place all assets directly in
`<configdir>/branding/` alongside `branding.json`.

Supported favicon formats are `.svg`, `.ico`, and `.png`. Any other extension is
treated as invalid and the built-in favicon is used instead.

If a `logo` filename is set but the file does not exist in `<configdir>/branding/`,
the server falls back to displaying the title text.

## Example

A minimal rebranding for "Acme Corp" might look like this:

```
<configdir>/branding/
  branding.json
  logo.svg
  favicon.svg
```

`branding.json`:

```json
{
  "title": "Acme Device Updates",
  "logo": "logo.svg",
  "favicon": "favicon.svg",
  "colors": {
    "primary": "#0b3d2e",
    "accent": "#7fd1ae"
  },
  "colorsDark": {
    "surface": "#12151c",
    "surface-alt": "#1a1f29",
    "text": "#e6e8ec"
  }
}
```

Effect: the top bar background becomes dark green, the custom logo SVG replaces
the title text in the navigation bar, the browser tab shows the custom favicon,
and page backgrounds and text color remain at their defaults in light mode because
`surface`, `surface-alt`, and `text` are not overridden in `colors`. In dark mode
the page background becomes `#12151c`, card and content areas become `#1a1f29`,
and the main text is rendered in `#e6e8ec`; the accent color `#7fd1ae` carries
over automatically as the dark-mode interactive/link color.

## Applying changes

Branding is loaded once when the server starts. After editing `branding.json` or
replacing any asset file, restart `fioserver` to pick up the changes.

## Defaults

| Key | Default value |
|---|---|
| `title` | `Foundries Update Server` |
| `logo` | *(none — title text is shown)* |
| `favicon` | *(built-in default favicon)* |
| `colors.primary` | `rgb(2, 11, 64)` |
| `colors.accent` | `#a3b4ff` |
| `colors.surface` | `#f2f2f2` |
| `colors.surface-alt` | `#ffffff` |
| `colors.text` | `#000000` |
| `colorsDark.surface` | `#11191f` |
| `colorsDark.surface-alt` | `#1a2632` |
| `colorsDark.text` | `#eef1f4` |

> [!IMPORTANT]
> Colors in `colors` apply to the light theme; colors in `colorsDark` apply to the
> dark theme. The UI automatically follows the browser or system light/dark preference —
> there is no manual toggle. Keep your `colorsDark` palette dark: the UI's small
> monochrome icons remain light-colored in dark mode by design and will not be
> legible against a light dark-surface.
