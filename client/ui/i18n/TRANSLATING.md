# Translating the desktop UI

Translations live in `client/ui/i18n/locales/<language>/common.json`. English
in `client/ui/i18n/locales/en/common.json` is the source of truth.

## Editing a translation

Copy the English file or edit the existing file for your language. Keep the
same keys. Missing keys fall back to English.

When a language is added for the first time, add it to
`client/ui/i18n/locales/_index.json` with its locale code, native display
name, and English name.

Run the parity check before committing:

```sh
node client/ui/i18n/check-translations.mjs
```

## Rules that keep the app working

- Copy placeholders such as `{version}`, `{count}`, and `{name}` exactly.
- Preserve `\n`, leading and trailing spaces, and trailing `...` where the
  English string uses them.
- Do not translate brand or product names such as NetBirdy, WireGuard, or
  Rosenpass.
- Keep acronyms such as DNS, IP, SSH, SSO, MFA, TCP, and UDP unchanged.
- Use the formal form of address where the language has one.
- Keep buttons and tray labels short. Long strings can wrap or truncate.

Plural support is limited to `one` and `other`. The singular key is used only
when `count` is 1. Every other value uses the plural key. Choose the form that
works best for the language within that limitation.

## Checking the result

Run the desktop app, switch to the language, and open the tray menu, main
window, settings tabs, and dialogs. Look for truncated text, broken
placeholders, inconsistent terms, and labels that do not match what the
control does.
