# Household

Other people can share an account's week (#238): children, a partner, anyone who eats the same meals with their own portions. They are managed from **Settings → Household** (`/settings/household`) by the account owner and have no login.

## How the quantities are worked out

- The account owner's plan is the base. A person's quantity of a row is the owner's times the ratio `person goal / owner goal` (both in kcal a day), rounded to a step that fits the unit: grams and millilitres to 5, pieces to half a piece, recipe servings to a quarter. A small quantity never rounds to zero.
- If the owner has no calorie goal in the profile the ratio is 1: everyone gets the owner's quantities, and the page says so.
- **By hand.** A person's quantity for one row can be set from the row (the chips under it open a dialog, one field for each person). It is an absolute quantity in the row's unit, so it stays when the owner changes the amount, and goes away when the owner changes the unit or replaces the ingredient (`household_item_quantities`). `0` means the person does not eat it; an empty field, or the automatic value, gives it back to the ratio.
- **Skipping a meal.** The meal's menu has «X skips this meal» for each person (`household_meal_skips`). A skipped meal counts for nothing for that person, in the day's kcal or on the shopping list.
- **Cheats.** A meal with a cheat counts what was eaten, scaled by the ratio; its rows are not editable per person.
- Never more than the owner: with a ratio under 1 the automatic quantity is capped at the owner's, so a pinch of salt is not rounded up to 5 g for a person with a smaller goal.
- **Copying a week** keeps what the people changed on their rows and meals.
- Reminders (#236) have no food and count for nobody.

## Where it shows

- The day: a bar for each person (kcal against their goal) in the summary, and a chip for each person on every row.
- The shopping list: one list, the quantities summed over everyone. `shopping_list_shares` keeps what each person needs of each ingredient (written when the list is generated; person 0 is the owner) for the «Who needs how much» detail. A list made before the household existed shows no detail until it is generated again, and a row's detail is hidden when it no longer adds up to the row (its quantity was edited, or a person came or went since).

## What it does not do yet

- A plan for each person that differs a lot, suggested meals and alternatives, and aggregated print views (separate issues).
- Replacing an ingredient for one person only: the person's quantity can be changed or set to 0, not swapped.
- The CSV export, the statistics, the REST API and the adherence score read the owner's plan only.
- A person cannot sign in or be linked to an account yet; coach mode is untouched.

## Code

`internal/household` (pure scaling, tested), `internal/db/household.go` and migration `0047_household.sql`, `internal/app/household_handlers.go` and `household_view.go`, the chips in `partials/day_card.html`, the dialog in `partials/household.html`, the page `pages/household.html`.
