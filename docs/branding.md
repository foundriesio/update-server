# Branding Your Update Server

You can visually rebrand the server without recompiling:
Drop your assets into a directory, edit a JSON file, and restart.

Branding files live in a `branding/` subdirectory of the server's data directory:

```
<configdir>/branding/
```

This directory is optional; if it is absent,
the server uses the default look with no further configuration needed.

> [!IMPORTANT]
> Branding is only read at startup.
> If `branding.json` is present but contains invalid JSON,
> the server logs a warning and falls back to the defaults.
> After any change to `branding.json` or your assets,
> restart the server for the changes to take effect.

## Config File

Add `branding.json` into `<configdir>/branding/`.
Every field is optional;
anything you omit, or that carries an invalid value, silently falls back to its default.

### Title and Images

> [!IMPORTANT]
> Files must be placed *directly* under `<configdir>/branding/`.
> When specifying the filename, it must not contain path characters (`/`, `\`),
> and must be in a supported format.
> Doing otherwise will result in a default being used.

* `title` (string) — Application name shown in the top navigation bar.
  Appended to every browser tab title.
  Example: a title of `Acme Device Updates` makes the Devices page read
  "Devices — Acme Device Updates".
  Default: `Foundries Update Server`.

* `logo` (string) — Filename of an image to display in the top navigation bar
  **in place of the title text**.
  Any browser-displayable image format is accepted (`.svg`, `.png`, `jpeg`, …).
  Default: title text

* `favicon` (string) — Filename of the browser tab and bookmark icon.
  Accepted formats: `.svg`, `.ico`, `.png`.
  Default: built-in favicon.

### Colors

> [!IMPORTANT]
> Colors in `colors` apply to the light theme,
> and colors in `colorsDark` apply to the dark theme.
> The UI automatically follows the browser or system light/dark preference.
> There is no manual toggle.
> Keep your `colorsDark` palette dark:
> the UI's small monochrome icons remain light-colored in dark mode
> and will not be legible against a light dark-surface.

* `colors` (object) — Light-mode theme colors.
  See `colorsDark` under [dark-mode](#dark-mode)
  Each value is a CSS color: hex `#rrggbb`, `rgb()` / `rgba()`, `hsl()`,
  or a CSS named color.
  Invalid values are ignored and the default is used.

  * `primary` — Top bar background and navigation accents.
    Default: `rgb(2, 11, 64)`.
  * `accent` — Hover and highlight color.
    Also used as the interactive/link color in dark mode.
    Default: `#a3b4ff`.
  * `surface` — Page background.
    Default: `#f2f2f2`.
  * `surface-alt` — Content and card background.
    Default: `#ffffff`.
  * `text` — Main text color.
    Default: `#000000`.

#### Dark Mode

* `colorsDark` (object, optional) — Dark-mode surface and text overrides.
  Mirrors the `colors` object in format, but only three keys are applied:
  `surface`, `surface-alt`, and `text`.
  The `primary` and `accent` brand colors are shared across both modes.
  Placing them inside `colorsDark` is accepted but has no effect;
  dark-mode interactive color is always derived from the `accent` value in `colors`.
  Each value follows the same CSS color formats and fail-soft validation as `colors`;
  Omitting `colorsDark` entirely, or omitting individual keys,
  keeps the built-in dark defaults for those fields.

  * `surface` — Page background in dark mode. Default: `#11191f`.
  * `surface-alt` — Content and card background in dark mode. Default: `#1a2632`.
  * `text` — Main text color in dark mode. Default: `#eef1f4`.

## Example Configuration

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

Effect:

* top bar background becomes dark green
* custom SVG logo replaces the title text in the navigation bar
* browser tab shows the custom favicon.

The page background and text color remain in the default light mode,
as `surface`, `surface-alt`, and `text` are not overridden in `colors`.

> [!TIP]
> In dark mode, the page background was set to `#12151c`,
> card and content areas became `#1a1f29`,
> and the main text was rendered in `#e6e8ec`.
> The accent color (interactive/link) remains `#7fd1ae`

## Applying Changes

Branding is loaded once when the server starts. After editing `branding.json` or
replacing any asset file, restart `fioserver` to pick up the changes.

## Summary of Defaults

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


