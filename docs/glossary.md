# Glossary and style rules

The terms Nutreek uses for its own ideas, with **English as the reference** and the
Italian used today (the two languages the project maintains) as an example of how
a translation settles each one. Use the same word for the same idea everywhere:
if "variant" is *variante* on one screen, it is never *alternativa* on another.

This page belongs to the [translation guide](translating.md).

## Terms

| English (reference) | Italian today | What it is, and what to watch |
|---|---|---|
| Week | Settimana | The main page: seven days of meals. Weeks start on Monday in every language. |
| Meal | Pasto | One eating occasion of a day: breakfast, lunch, a snack. It holds foods. |
| Meal type | Tipo pasto | The kind of a meal (Breakfast, Lunch…) that a nutrition plan gives variants to. |
| Variant | Variante | One way to make a meal in the nutrition plan (the *main variant* is the default one). Week generation picks among variants. Not "option" or "alternative". |
| Weekly template, template | Template settimanale, template | The scheme of the week's meals: which meals exist on which days. Italian keeps the English loanword; translate it if your language has a natural word. |
| Nutrition plan | Piano nutrizionale | The set of meals and their variants a person follows. A coach can assign one. Not the same as the *week* (what you actually plan). |
| Cheat | Sgarro | A meal outside the plan, logged on purpose ("Log a cheat"). Cheats do not count in the shopping list. Keep a light, non-judgmental word. |
| Macros | Macro | Protein, carbs and fat taken together. |
| Protein / Carbs / Fat | Proteine / Carboidrati / Grassi | The three macros. "Carbs" is the short form: use whatever is short enough for a column header. |
| P / C / F | P / C / G | The one-letter forms in tight places (week view, tables). Use your language's initials as on a food label. |
| kcal | kcal | Energy. Not translated. Spelled "calories" only in `common.calories`. |
| Serving | Porzione (porz.) | One portion of a recipe or product, as the recipe or the label defines it. Servings of a recipe are counted ("2 of 4 servings"). |
| Piece | Pezzo (pz) | One countable unit of a food, such as one apple. Short form `pc` in English, `pz` in Italian: it appears next to quantities. |
| Quantity | Quantità | An amount of a food in a meal: grams, millilitres or pieces. |
| Ingredient | Ingrediente | A food in the user's library (also used for products found in OpenFoodFacts). Appears as the section name *Ingredients*. |
| Recipe | Ricetta | Ingredients combined, with servings. |
| Shopping list | Lista della spesa | Built from the week's meals and recipes. Short form *Shopping* (`layout.nav.shopping_short`) in the phone's bottom bar. |
| Coach | Coach | A person who manages plans for clients. Italian keeps the English word. *Coach panel* is the coach's section. |
| Client | Cliente | A user a coach follows. |
| Premium | Premium | A paid tier that unlocks some features. Keep the word; it is a product name. |
| Daily needs (TDEE) | Fabbisogno (TDEE) | Estimated energy the body uses in a day. TDEE is the standard abbreviation; keep it next to your word. |
| Goal | Obiettivo | A daily target: kcal, macros, water. *Maintenance*, *Weight loss* and *Muscle gain* (*Mantenimento*, *Dimagrimento*, *Massa*) are the goal presets. |
| Weigh-in | Pesata | One weight entry. *Weight* (*Peso*) is the section. |
| Water | Acqua | Daily hydration log. |
| Workout | Allenamento | An exercise session that burns kcal. Activity names (Running…) are in `reference.json`. |
| Allergen | Allergene | An allergy marker on an ingredient. Names are in `reference.json`. |
| Statistics | Statistiche | The charts section. |
| Settings | Impostazioni | The user's settings page. |
| Administration | Amministrazione | The owner's panel. |
| Guided tour / Quick guide | Tour guidato / Guida rapida | The first-run walkthrough, and the help page. |
| Appearance: Light / Dark / System | Aspetto: Chiaro / Scuro / Sistema | Colour mode. |
| Colours: Forest, Sea, Sunset, Graphite | Bosco, Mare, Tramonto, Grafite | Names of the colour themes. Translate the meaning, not letters. |
| Log in / Log out / Sign up | Accedi / Esci / Registrati | Verbs on buttons. |

If a term has no natural translation in your language, keeping the English word is
fine and common (Italian does it for *template*, *coach* and *Premium*). Say so in
the pull request.

## Style rules

**Tone.** Friendly and direct, never chatty. Speak to the person ("your week",
"you have no templates yet") in the form your language uses for an app, usually the
informal one; Italian uses *tu*. Say what happened and what to do next. Do not
blame the user for an error, and do not apologise at length.

**Capitalisation.** Sentence case: only the first word and proper names start with a
capital letter (*Add meal*, *Nutrition plan*), including buttons, menu items and
headings. Keep your language's own rules for nouns where they differ (German).

**Buttons start with a verb**, in the imperative or the infinitive your language uses
for commands: *Add meal*, *Create template*, *Delete variant*. A destructive action
names what it deletes. The confirmation question uses the same word as the button.

**Short labels.** Buttons, chips, tabs, table headers and the phone's bottom bar have
room for one or two words. Prefer the shorter synonym. Where English has a `_short`
key (`layout.nav.shopping_short`, `common.protein_short`), it is there because the
long form does not fit: translate it with something that really is short.

**Punctuation.** Full sentences end with a full stop; labels, buttons, titles and
placeholders do not. Use the quotation marks of your language for `{{.Name}}` in
dialogs (English “…”, Italian «…»). Keep `…` (one character) on actions that open a
dialog, as English does (*Rename…*). Spaces before `:`, `?` or `!` follow your
language's rule (French has them).

**Units.** The symbols `g`, `ml`, `kg`, `cm`, `kcal` and `%` are never translated, and
a space separates them from the number, as in English (`150 g`, `1,894 kcal`). Do not
write the unit twice if a placeholder already has it: check the English text.

**Numbers and dates.** A number is a placeholder (`{{.Kcal}}`): the app formats it,
you only place it. Do not write digits for numbers that the English text spells as a
word ("one"). Dates are built from month and weekday names in `format.json` and an
order in `format.range.*` and `format.short_date`: set the order your language uses
(*Oct 12* in English, *12 ott* in Italian). See also *Known limits* in the
[translation guide](translating.md#known-limits).

**Consistency over cleverness.** Reuse the glossary word everywhere, even where a
synonym would read better. A reader who sees two words assumes two features.

**Errors and messages.** Messages that start with a lower-case word (for example
`week.error.meal_not_found`) are internal error texts that reach the screen rarely;
translate them anyway, in the same plain tone.
