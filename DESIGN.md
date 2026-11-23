---
version: alpha
name: Nutreek
description: Weekly meal planning and nutrition tracking, used daily on a phone and at a desk.
colors:
  background: "#FFFFFF"
  surface: "#FFFFFF"
  surface-muted: "#F0F2F5"
  border: "#E3E6EA"
  rule: "#1F2328"
  text: "#1F2328"
  muted: "#5D6570"
  track: "#ECEEF1"
  primary: "#2F7A55"
  primary-strong: "#1F5C3E"
  primary-soft: "#E3F1E9"
  on-primary: "#FFFFFF"
  danger: "#B3261E"
  danger-soft: "#FCE8E6"
  protein: "#6B5BD2"
  carbs: "#C98A12"
  fat: "#23897D"
typography:
  title:
    fontFamily: Figtree
    fontSize: 22px
    lineHeight: 28px
    fontWeight: 700
  heading:
    fontFamily: Figtree
    fontSize: 17px
    lineHeight: 24px
    fontWeight: 600
  body:
    fontFamily: Figtree
    fontSize: 15px
    lineHeight: 22px
    fontWeight: 400
  small:
    fontFamily: Figtree
    fontSize: 14px
    lineHeight: 20px
    fontWeight: 400
  caption:
    fontFamily: Figtree
    fontSize: 12px
    lineHeight: 16px
    fontWeight: 500
rounded:
  base: 6px
  full: 9999px
spacing:
  sm: 8px
  md: 12px
  lg: 16px
  xl: 24px
components:
  button-primary:
    backgroundColor: "{colors.primary}"
    textColor: "{colors.on-primary}"
    rounded: "{rounded.base}"
    height: 44px
    padding: 0 16px
  button-secondary:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.text}"
    rounded: "{rounded.base}"
    height: 44px
    padding: 0 16px
  chip:
    backgroundColor: "{colors.primary-soft}"
    textColor: "{colors.primary-strong}"
    rounded: "{rounded.full}"
---

## Overview

Nutreek is a self-hosted web app for planning meals by the week and tracking what was eaten. People open it several times a day, mostly on a phone, to log a meal, check what is left of the day's calories, and tick the shopping list. The look is a ledger: content sits directly on the page, in rows and columns separated by rules, with numbers that line up. Nothing decorates; the day's data is the interface.

The same layout works on a phone and on a desktop. A screen is designed once for both widths, never for one first.

## Colors

`primary` is the colour of actions and of the current place (active navigation, meal times). Four accents exist (Bosco, Mare, Tramonto, Grafite) and the user picks one; every accent keeps the same roles, so a screen never depends on the green. `primary-strong` is the only colour for text placed on `primary-soft`.

`protein`, `carbs` and `fat` mark macro bars and legend dots and nothing else. A macro number is written in `text`, because `carbs` fails contrast as text. Over-target is shown with `danger`, always together with the number, never by colour alone.

Every text and control colour meets WCAG 2.2 AA (4.5:1 for text under 24px, 3:1 for large text and component borders) in light and dark.

## Themes

Light is the default. Dark is chosen on the device or in settings.

| Token | Dark | Mare | Tramonto | Grafite |
|---|---|---|---|---|
| background | #16181C | | | |
| surface | #1E2126 | | | |
| surface-muted | #272B31 | | | |
| border | #30343B | | | |
| rule | #E6E8EB | | | |
| text | #E6E8EB | | | |
| muted | #A0A7B2 | | | |
| track | #2B2F36 | | | |
| primary | #5FBF8C | #1D6FA5 (dark #5AA9E0) | #B4511A (dark #F08A4B) | #3A4150 (dark #C9CED6) |
| primary-soft | #1F3329 | #E1EEF8 (dark #1B2C3A) | #FBE9DD (dark #3A2619) | #E7E9EE (dark #2C3038) |
| on-primary | #0F1216 | | | |
| danger | #FF8A80 | | | |
| danger-soft | #3A2022 | | | |
| protein | #A99DF0 | | | |
| carbs | #E9B851 | | | |
| fat | #5CC9BB | | | |

## Typography

Figtree throughout; no second family. Every number is set with tabular figures so columns of quantities and kcal align down the page. `title` names the screen (a day, a week), `heading` names a section (a meal, Water, Workout), `body` is rows and forms, `small` is secondary text and metadata, `caption` is labels under a control. Never go below 12px.

## Layout

A section is a heading line with a rule above it; its content is rows. A strong rule (`rule`, 2px) opens each main section of a screen, a hairline (`border`, 1px) separates rows inside it. Do not wrap content in boxes, and never put a box inside a box.

Rows are one line: name, then quantity, then the value, right-aligned in tabular figures. Macros are shown once per meal, not repeated per row. Names wrap instead of truncating.

Touch targets are at least 44px on a phone and 40px on a desktop, with 8px or more between them. Navigation has four destinations, the same on every width: Week, Shopping, Library, Progress. On a phone they are a bar at the bottom; on a desktop, a sidebar. Settings, guide and sign-out open from the person's avatar.

One "Add" button per screen starts every creation (meal, reminder, workout). A list's own "Add" links are for adding to that list only.

## Elevation & Depth

Flat. Only floating layers (menus, dialogs, the install banner) have a shadow, and always with a border.

## Shapes

Controls use `rounded.base`. Avatars, chips and day pills use `rounded.full`. Panels with a fill do not exist; a surface is a popover, a dialog or an input.

## Components

Primary action: one `button-primary` per screen region. Everything else is `button-secondary` or a text link in `primary-strong`. A meal time is `primary`-coloured text, in tabular figures, to the left of the meal name.

## Do's and Don'ts

- Do keep a data screen to one level of emphasis: the strong rule and the day's total are the loudest things on it.
- Don't use gradients, glass blur, emoji or decorative icons; icons are outlined strokes that carry meaning next to a label.
- Don't colour text with a macro colour.
- Don't show a negative sign for energy burned in a workout; write "burned about 300 kcal (estimate)".
