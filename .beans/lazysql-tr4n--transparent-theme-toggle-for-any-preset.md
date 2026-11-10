---
# lazysql-tr4n
title: Transparent theme toggle for any preset
status: completed
type: feature
priority: normal
parent: lazysql-vq7e
created_at: 2026-11-10T17:18:40Z
updated_at: 2026-11-10T17:18:40Z
---

Only the default preset used the terminal background. Every other preset painted its own.

- [x] `[theme] Transparent = true` forces the terminal background on any preset
- [x] Theme picker toggles it with `t` and saves it with the preset
- [x] Runtime recolor handles painted -> transparent -> painted
- [x] README and CONTEXT.md

## Summary of Changes

- `ThemeConfig.Transparent` overrides `PrimitiveBackground` with the terminal default after preset and color overrides.
- `Config.SaveTheme(preset, transparent)`; `SaveThemePreset` kept as the non-transparent form.
- Theme picker previews and saves the toggle.
- Tests for every preset, saving, and live toggling. Manual check of modal readability is left to the user.
