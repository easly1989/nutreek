---
name: New language
about: Add a translation of the Nutreek interface, or claim a language
title: "i18n: add <language>"
labels: enhancement
---

<!--
Thank you! Nobody else needs to be asked before you start; this issue only
makes sure two people do not translate the same language. The guide:
docs/translating.md. The words to use: docs/glossary.md.
-->

## Language

- Name, in English and in the language itself:
- Code (BCP 47, for example `de`, `pt-BR`):
- Do you speak it natively? (yes / no)
- Will you translate it yourself? (yes / looking for help)

## Checklist

- [ ] I read [the translation guide](https://github.com/easly1989/nutreek/blob/main/docs/translating.md) and the [glossary](https://github.com/easly1989/nutreek/blob/main/docs/glossary.md)
- [ ] `internal/i18n/locales/<code>/` is a copy of `locales/en/`, with the values translated and the keys untouched
- [ ] One line added to `Languages` in `internal/i18n/i18n.go`, with the native name and `Beta: true`
- [ ] `go run ./cmd/i18ncheck` shows no errors (warnings for keys not translated yet are fine)
- [ ] I tried the language in the app: Week, Nutrition plan, Recipes, Ingredients, Shopping list, Settings, the sign-in pages
- [ ] Screenshots in the pull request: desktop and phone, demo data only
- [ ] A line in `CHANGELOG.md` under *Unreleased*
- [ ] I will update the language when new keys appear (CI lists them as warnings)

## Notes

Anything you want to settle before translating: a term with no natural word in
your language, a text that does not fit a button, a number or date format that
looks wrong.
