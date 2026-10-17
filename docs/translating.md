# Translating Nutreek

Anyone can add a language to Nutreek with a pull request. You do not need to read
the code, and you do not need to know Go: a translation is a few JSON files plus
one line in a list. This guide is everything you need.

Italian and English are written and kept complete by the maintainer. Every other
language is a **community language**: it is marked **beta** in the language
pickers, and a key you have not translated yet simply shows the English text.
Nobody is blocked by a missing key, and CI only warns about it (see
[Checking your work](#checking-your-work)).

The words Nutreek uses for its own ideas (week, meal, variant, template, cheat…)
are in the [glossary](glossary.md), with the style rules. Read it before you
start: it saves a round of review.

- [What you need](#what-you-need)
- [Add a language in six steps](#add-a-language-in-six-steps)
- [What the files look like](#what-the-files-look-like)
- [Try it locally](#try-it-locally)
- [Checking your work](#checking-your-work)
- [What not to translate](#what-not-to-translate)
- [Tight spots to look at](#tight-spots-to-look-at)
- [The pull request](#the-pull-request)
- [Keeping a language up to date](#keeping-a-language-up-to-date)
- [Known limits](#known-limits)

## What you need

- A GitHub account, and `git`.
- [Go](https://go.dev/dl/) 1.27 or later. It runs the check and the app. Nothing
  else is required to translate; a one-off download of the Tailwind CLI makes the
  app look right (see [Try it locally](#try-it-locally)).
- A text editor that saves UTF-8 files.

Tell us you are on it first, if you like: open an issue with the **New language**
template, so two people do not translate the same language.

## Add a language in six steps

Pick the language code (BCP 47): `de`, `fr`, `pt-BR` for Brazilian Portuguese.
The examples below use `de`.

1. **Fork** the repository and create a branch, for example
   `feat/i18n-de`.
2. **Copy the English files.** Copy the folder `internal/i18n/locales/en/` to
   `internal/i18n/locales/de/`. Keep the file names: one file per area of the app
   (`week.json`, `plan.json`, `settings.json`…).
3. **Translate the values**, not the keys. See
   [What the files look like](#what-the-files-look-like). You do not have to do
   all of them in one go: a partial translation is a valid pull request, and the
   rest shows in English.
4. **Add one line** to the `Languages` list in `internal/i18n/i18n.go`, with the
   language's own name and `Beta: true`:

   ```go
   var Languages = []Language{
       {Code: "it", Name: "Italiano"},
       {Code: "en", Name: "English"},
       {Code: "de", Name: "Deutsch", Beta: true},
   }
   ```

   The `Code` is the folder name. The `Name` is how speakers of the language call
   it (`Deutsch`, not `German`; `Português (Brasil)`).
5. **Check it**: `go run ./cmd/i18ncheck` (see
   [Checking your work](#checking-your-work)).
6. **Try it** in the app and take screenshots (see
   [Try it locally](#try-it-locally) and [The pull request](#the-pull-request)),
   then open the pull request.

You never change anything else: no templates, no Go code, no other language's
files.

## What the files look like

Each file is a flat JSON object of `"key": "text"`. The key starts with the name
of the file, and **keys are never translated**:

```json
{
  "week.add_meal": "Add meal",
  "week.prev": "Previous week"
}
```

becomes

```json
{
  "week.add_meal": "Mahlzeit hinzufügen",
  "week.prev": "Vorherige Woche"
}
```

Translate for what the text does on the screen, not word by word. A button says
what it will do ("Add meal"); a label names the thing.

### Placeholders

`{{.Name}}` marks a value Nutreek fills in: a number, a name, a date.

```json
"week.goal": "goal {{.Kcal}} kcal"
```

- Copy each placeholder **exactly as written**, braces and capital letter
  included, and keep every one: `"Ziel {{.Kcal}} kcal"`. Never translate the word
  inside the braces.
- You can move a placeholder to wherever your language needs it: dates in
  English read `{{.Month}} {{.Day}}`, in Italian `{{.Day}} {{.Month}}`.
- You can use the same placeholder twice. You cannot add one the English text
  does not have; the check fails if you do.

### Plurals

When a number changes the wording, the English value is an object with the plural
forms instead of a string:

```json
"plan.variants": {
  "one": "{{.Count}} variant",
  "other": "{{.Count}} variants"
}
```

Write the forms **your language** uses, which may be more or fewer than English:
`zero`, `one`, `two`, `few`, `many`, `other` (the
[CLDR plural rules](https://cldr.unicode.org/index/cldr-spec/plural-rules) list
them per language; Polish needs `one`, `few`, `many`, `other`, Japanese only
`other`). `other` is always required. Keep `{{.Count}}` in the forms that show
the number.

### Quotes and characters

Use the quotation marks of your language. Italian writes «…» where English writes
“…”. Keep the ellipsis `…` as one character where English has it. A literal
double quote inside a value is written `\"`. Do not add HTML to a value.

## Try it locally

You can see your translation in the real app with Go alone. The app embeds the
catalogs when it is built, so **restart it after every edit** (stop with
`Ctrl+C`, run again).

### 1. Start the app

From the repository root, in a terminal (PowerShell on Windows, any shell on
macOS and Linux):

```
go run ./cmd/nutreek
```

The first run downloads the Go dependencies, creates a local SQLite database in
`./data/` (ignored by git) and serves on <http://localhost:8080>. If the port is
busy, pick another one: `NUTREEK_ADDR=:8081 go run ./cmd/nutreek` on macOS and
Linux, `$env:NUTREEK_ADDR=":8081"; go run ./cmd/nutreek` in PowerShell.

At this point the app works, but **without its stylesheet**: the stylesheet is
generated, not stored in the repository, so a fresh clone shows plain unstyled
pages. That is enough to read your texts; for screenshots and for judging how a
text fits a button you want the real look, so do step 2 once.

### 2. Build the stylesheet (once; again only if you change templates)

This downloads the Tailwind CLI v3.4.17 (a single executable) into `./bin/`
(ignored by git) and builds `internal/web/static/css/tailwind.css`.

- **Windows (PowerShell)**

  ```powershell
  pwsh scripts/setup.ps1
  pwsh scripts/tailwind.ps1
  ```

  `powershell scripts/setup.ps1` (Windows PowerShell 5.1) works too.

- **macOS or Linux with `make`**

  ```bash
  make setup
  make tailwind
  ```

- **macOS or Linux without `make`**: download the binary that matches your
  machine (`linux-x64`, `linux-arm64`, `macos-x64` or `macos-arm64`; this example
  is Linux on Intel/AMD), then build:

  ```bash
  mkdir -p bin
  curl -sSL -o bin/tailwindcss https://github.com/tailwindlabs/tailwindcss/releases/download/v3.4.17/tailwindcss-linux-x64
  chmod +x bin/tailwindcss
  bin/tailwindcss -c web/tailwind/tailwind.config.js -i web/tailwind/input.css -o internal/web/static/css/tailwind.css --minify
  ```

Restart `go run ./cmd/nutreek` afterwards: the stylesheet is embedded too. Only
the Go toolchain and this one download are needed.

### 3. Switch to your language

- **Signed out**: the sign-in and sign-up pages have a button per language at the
  bottom. Your language is there, with a small *beta* mark.
- **Signed in**: sign up with any email address (nothing is sent anywhere; the
  account lives only in your local `./data/` database), then open
  **Settings → Language** and choose yours.

Walk through the app: Week, Nutrition plan, Recipes, Ingredients, Shopping list,
Statistics, Weight, Settings, and the sign-in pages. Add a few meals and a recipe
so the screens have content. Text you typed yourself (names of foods, meals,
recipes) is never translated.

To see the screens as a phone does, narrow the browser window or use the browser's
device toolbar (`F12`, then `Ctrl+Shift+M`).

## Checking your work

```
go run ./cmd/i18ncheck
```

or, as CI runs it, together with the other tests: `go test ./internal/i18n`
(add `-v` to see the warnings).

**Errors** make the check fail, for every language:

- a file that is not valid JSON, or a value that is empty;
- a key that does not start with the name of its file;
- a key that does not exist in English (a typo in the key, or a key that was
  renamed or removed since you copied the files);
- a placeholder that English does not have, or one that is missing;
- a plural object without `other`;
- a folder in `locales/` that is not in the `Languages` list (you forgot step 4).

**Warnings** do not fail anything. They list the keys of your language that are
still missing, per file, and the app shows the English text for them:

```
warning: de: 1114 keys to translate (English is shown until then)
  locales/de/admin.json
    admin.features.active_label
    admin.features.col_active
  …
ok: no errors (the warnings above do not fail the build)
```

On GitHub the same warnings appear as annotations on the CI run of your pull
request. Italian and English are different: for them a missing key is an error,
so a maintainer adding a feature always updates both.

## What not to translate

- **Keys** (`"week.add_meal"`), and anything inside `{{ }}` braces.
- **Brand and product names**: Nutreek, OpenFoodFacts, and the names of formulas
  such as Mifflin-St Jeor.
- **Units and symbols**: `g`, `ml`, `kcal`, `kg`, `cm`, `%`. (The word for "piece"
  is translated, see the glossary; the unit symbols are not.)
- **What users typed**: ingredient, recipe and meal names, notes. They are data,
  not interface text, and are never in the catalogs.
- **OpenFoodFacts product names**: they come from the public database, as written
  there.

The files `reference.json` hold the allergens and the workout activities that
Nutreek creates itself. They are interface text: translate them like the others.

## Tight spots to look at

Open these screens in your language, on a desktop and on a phone, and make sure
nothing is cut off, wraps badly or overlaps:

- **Buttons and chips**: a long word does not fit in a small button. Prefer a
  shorter synonym; the glossary lists the short forms Italian uses.
- **The bottom bar on a phone**: the section names (`layout.nav.*`) share a narrow
  row. `layout.nav.shopping_short` exists for exactly this reason.
- **Macro letters**: `common.protein_short`, `common.carbs_short` and
  `common.fat_short` are single letters in the week view and the tables (English
  P, C, F; Italian P, C, G). Use the initials your language uses on food labels.
- **Dates**: month and weekday names, and the order of day and month, are all in
  `format.json`. Check the title of the week, for example *Week of October 12–18*.
- **Confirmation dialogs** and the **guided tour** (`tour.json`): they have the
  longest texts.

If a text is too long and you cannot shorten it, say so in the pull request; it is
a design problem, not a translation problem.

## The pull request

One language, one branch, one pull request to `main`.

- **Commit message** in English, Conventional Commits:
  `feat(i18n): add German (beta)`.
- **Changelog**: add a line under *Unreleased → Added* in `CHANGELOG.md`, for
  example `- German translation (beta) (#NN)`.
- **Screenshots**, in the pull request description: the interface in your
  language, on desktop and **on a phone**. At least:
  1. the sign-in page with the language picker (showing the *beta* mark);
  2. the **Week** page with a few meals;
  3. **Settings → Language**;
  4. the **Nutrition plan** page;
  5. the **Shopping list**;
  6. one more page where you had to shorten something.
  Drag the images into the description box on GitHub. Use made-up demo data only:
  no real email address, name or weight in any picture.
- Say in the description how complete the translation is (all files, or which
  ones) and paste the last lines of `go run ./cmd/i18ncheck`.

CI must be green. A maintainer cannot judge your language, so reviewing is about
the checks and the screenshots; if another speaker of the language is around,
ask them to have a look.

## Keeping a language up to date

Whoever contributed a language keeps it up to date with further pull requests;
the maintainer does not translate it. When Nutreek gains a feature, new keys are
added to English and Italian, and your language shows the English text for them
until you translate them. The CI warning on any pull request lists exactly which
keys are missing. When you are happy that a language is complete, ask in an issue
whether the *beta* mark can go.

## Known limits

- **Number format.** Thousands and decimal separators are not in the catalogs:
  English gets `1,894` and `13.5`, every other language currently gets the
  Italian style, `1.894` and `13,5`. If your language writes numbers differently,
  open an issue; it needs a small code change, not a translation.
- **The week starts on Monday** in every language.
- **Right-to-left languages** (Arabic, Hebrew…) have not been tried: the layout is
  not mirrored.
- **The REST API** answers in English whatever the language.
