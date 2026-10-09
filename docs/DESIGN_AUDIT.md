# Design audit

Scope: the signed-in app at 1280 px wide, light theme, graphite accent, branch `pages-design` at `9d10c13`.
Pages measured: Chat, Files, Agents, Activity, Spending, Conversation, Record, Admin overview.
Dark theme, the folder-choose dialog, the file viewer and the other admin pages are not measured.

## How it was measured

A script walked every visible element on each page in the browser pane and read computed styles.

| Measure | How |
| --- | --- |
| Type | Distinct size / weight / family of every element that owns text |
| Colour | Distinct text colours; WCAG contrast of each text element against its nearest opaque background |
| Radius, spacing | Distinct `border-radius`, `gap`, `padding-left`, `padding-top`, `margin-top` |
| Icons | Rendered width and `stroke-width` of every `svg` |
| Targets | Rendered height of every link, button, input, select and summary |

Severity: **P0** breaks a rule the product states about itself. **P1** is visible to a user. **P2** is polish.

## 0. Totals across the eight pages

| Atom | Found | Should be | Verdict |
| --- | --- | --- | --- |
| Font sizes | 11, 12, 13, 14, 15, 16, 18, 24, 28 px (9) | 6 steps | Too many; 13, 14 and 15 sit side by side in one list |
| Type styles per page | 7 to 11 (size / weight / family) | 6 to 7 | Activity has 10, Spending has 11 |
| Text colours per page | 3 to 5 | 3 to 4 | Good. `#55554F` and `#66665F` are one step apart and read as the same |
| Radii | 3, 8, 9, 12, 18, 20, 24 px, 999 px, 50% | 4, 8, 12, full | 9, 18, 20, 24 and 3 are off the scale |
| Spacing values | 1, 2, 4, 8, 12, 15, 16, 20, 24, 32, 36 px | 4-point steps | 15 px (Spending) is off the grid |
| Icon sizes | 13, 14, 15, 16, 18, 20, 22 px | 16, 20, 24 | Seven sizes |
| Icon stroke | 1.75 everywhere except 1.5 (Files list, 9 icons) and 2 (send) | one value | Three values |
| Control heights | 15, 17, 18, 19, 20, 23, 24, 28, 29, 30, 32, 36, 37, 38, 40, 44, 48 px | 28, 36, 44 | 17 distinct heights |
| Contrast failures | 1, the decorative `/` in the Files heading at 1.48:1 | 0 for text | Pass |
| Horizontal overflow | 0 | 0 | Pass |

## 1. Foundations

### 1.1 Colour

- Four surface layers exist and read in order: rail, conversations panel, page, card.
  Each is one step from its neighbour in light and in dark.
- Accent use is restrained: one filled action per page and the place marker on the rail.
- **P1** Muted (`#55554F`) and subtle (`#66665F`) text differ by 7 luminance points.
  Pages use both in the same row (Activity: 33 and 14 elements), so the difference carries no meaning.
  Merge to one muted value, or make subtle visibly lighter and use it only for metadata.
- **P2** Status colour is a 6 px dot plus a word on Activity and Admin, and a green word on Record.
  One treatment is enough: dot plus word.

### 1.2 Type

- Two families: Sora for display, Noto Sans for text. The split is sound.
- **P0** The headings are not one system.
  Page title is Sora 24 (Files, Agents, Activity, Spending, Admin), Sora 18 (Record), Noto 15 (Conversation header), Sora 28 (Chat hero), Sora 16 (panel name, brand).
  Define four roles: display 28, page title 24, section 16, label 13.
- **P1** The body has three adjacent sizes: 13, 14 and 15 px.
  Rows use 15, cells 14, metadata 13, and sidebar chats 14.
  Collapse to 14 for content and 12 or 13 for metadata.
- **P1** Two table-head styles: uppercase 12/600 with tracking (Agents, Activity, Spending) and sentence case 13/500 (Files).
  Pick the Files one.
- **P1** Eleven-pixel text exists on Spending (chart day labels, 9 elements) and Record (5 uppercase labels).
  Floor is 12 px.
- **P2** Numerals are tabular in the figure strip only. Use tabular figures in every numeric column.

### 1.3 Spacing and radius

- Components snap to a 4-point grid apart from 15 px on Spending.
- Radii: cards 12, controls 8 are used consistently.
  The odd ones are the composer (20 px), the user bubble (18 px), the chart bars (3 px) and the agent avatar (9 px).
  Move to 4 / 8 / 12 / 16 / full and map each odd one to the nearest.

### 1.4 Icons

- One set, Lucide, on a 24-unit grid.
- **P1** Seven rendered sizes.
  Use 20 px in controls and navigation, 16 px inline with text, 24 px for empty states.
- **P1** Stroke is 1.75 except the Files list (1.5, set by `.ex .ic svg`) and the send arrow (2).
  Remove both overrides.
- **P2** The rail and the page do not share a size. Rail icons are 22 px; page toolbar icons are 18 px.

### 1.5 Elevation and motion

- Light cards use `--yg-shadow-1`; dark uses borders only. Correct.
- The appearance panel and menus use shadow 3. Correct.
- **P2** Hover has no transition on rows or rail tools. Add 120 ms on background and colour.

## 2. Shell

### 2.1 Rail

- Width 4 rem, tool 44 px, icon 22 px, marker 3 × 20 px. Hit areas meet the 44 px target.
- **P1** No labels. Seven icons without text need hover tips, which exist, but the desktop app's first-run user has no hint.
  Add a one-time label expansion, or show labels under icons at wide widths.
- **P1** The toggle, Chat and the brand mark are visually the same weight, so the product has no anchor.
  Separate the mark from the tools with more space and make it non-interactive-looking.
- **P2** Admin shows only to admins and has no marker when active (`aria-current` is not set on it).

### 2.2 Conversations panel

- Width 16 rem fixed.
- **P1** Rows are 30 px (chats) and 32 px (projects), under the 36 px minimum for a primary list.
- **P1** The same three titles repeat under every project in test data, so users cannot tell chats apart.
  Show a relative time on the right of each chat and truncate with a tooltip.
- **P2** The panel name "Latentic Agent Cloud" repeats the brand mark. Replace with the organisation name.
- **P2** "Older projects" fold has no count affordance beyond a number.

### 2.3 Page frame

- Content max 72 rem, padding 2 rem, centred. Chat is full bleed.
- **P1** Pages with a table fill the width, while Admin overview centres a narrower column.
  Pick one measure for list pages.
- **P1** Page title rows use three different minimum heights (Files 40, Agents 40, Record auto).

## 3. Chat

- Hero heading Sora 28, composer 20 px radius, one send button.
- **P1** The composer's row mixes a text control (Folder), two selects (agent, model) and the send button at three heights (24, 28, 36 px).
  Use one control height of 32 px for the row.
- **P1** The model label "DeepSeek Flash" has no icon or provider cue, and the agent select reads "Software engineer".
  Neither says it is a select. Add a chevron on both.
- **P2** No suggestion row. A new user has one text box and a heading.

## 4. Conversation

- **P0** The header title is Noto 15 where every other page uses Sora.
- **P1** Turn metadata ("0s · Record") is 12 px muted and sits 4 px under the bubble. It reads as an error line.
- **P2** The user bubble radius (18) matches nothing else.

## 5. Files

- Heading is the path; list is one card; icons by kind.
- **P1** Icon stroke 1.5 against 1.75 everywhere else (see 1.4).
- **P1** Folder rows have a chevron but file rows have none, so the right edge is ragged.
- **P1** Size is blank for folders. Show an item count.
- **P2** The `/` separators measure 1.48:1. They are decorative, so exempt, but they vanish in bright light.
- **P2** No sort, no multi-select, no drag. Not needed for the pilot.

## 6. Agents

- **P1** "Compare" (32 px) sits beside "New agent" (36 px). Adjacent actions must share a height.
- **P1** The avatar tile radius (9 px) and letter size differ from every other identity mark.
- **P1** The third column header "Latest conversation" wraps below 56 rem.

## 7. Activity

- Densest page: 10 type styles, 50 controls, 35 under 32 px.
- **P1** The filter chips (Everything, Running, Queued, Passed, Failed) are 28 px tall and the `every engine` select beside them is 37 px.
- **P1** Each row repeats the agent name in bold, then the brief, then "replied" and a duration at the right.
  The status and time are 13 px muted at the far right, 600 px from the title. Pull status next to the title.
- **P2** The date group label "TODAY" uses the uppercase style that Files dropped.

## 8. Spending

- **P1** The first figure tile (`.figure.headline`) has a 30 px number in the accent-text colour and a gradient from the accent surface (`app.css:605`).
  The other four are 24 px ink on white. With graphite the gradient is grey, so the first tile reads as dimmer than its neighbours.
  Give it the same white background and mark it by size alone.
- **P1** Chart labels are 11 px.
- **P1** Four panels hold the sentence "Nothing this month." with no illustration or next step.
  An empty state needs one line and, where useful, an action.
- **P2** 15 px spacing appears 4 times. Snap to 16.

## 9. Record

- **P1** The breadcrumb is 11 px uppercase for "RECORD · MESSAGE 1 OF 1" and 13 px for the trail above it.
  Two levels of orientation with different styles.
- **P1** The outcome chip uses a green on pale green. It is the only coloured chip in the app and it is 11 px.
- **P2** Cards "What changed" and "Network" share the same empty sentence form. Fine, but both lack a visible count.

## 10. Admin

- **P1** The first figure tile (`.figure.headline`) has the same accent-surface gradient, which is grey under graphite while its neighbours are white.
  It reads as disabled. The rule is the same one that affects Spending.
- **P1** The console is a different frame from the app: top bar, no rail, section list on the left at 12 rem.
  It is deliberate, but the section list is 13 px 500 where the app's panel is 14 px 500.
- **P2** The "Back to Latentic Agent Cloud" link and the sign-out icon are 40 px apart with no divider.

## 11. Appearance panel

- Theme is a three-way segmented control; accent is six 26 px swatches.
- **P1** Swatches are 26 px; the minimum target is 32 px. Raise to 32 px and keep the ring.
- **P2** No preview of what a swatch looks like on a button. The ring and the page change are the feedback.
- **P2** The choice is per device. It does not follow the person to a second device.

## 12. Order of work

| # | Fix | Severity | Effort |
| --- | --- | --- | --- |
| 1 | One heading system: four roles, applied to every page and the conversation header | P0 | Small |
| 2 | One control height set (28, 36, 44) and apply to buttons, chips, selects, composer row | P1 | Medium |
| 3 | One type scale (12, 13, 14, 16, 24, 28) with a floor of 12 px | P1 | Medium |
| 4 | Icons to 16 / 20 / 24 with one stroke | P1 | Small |
| 5 | Radii to 4 / 8 / 12 / 16 / full | P2 | Small |
| 6 | Table head style, one for every list | P1 | Small |
| 7 | Chat rows in the panel: time on the right, 36 px rows | P1 | Small |
| 8 | The `.figure.headline` tile on Spending and Admin: white background, size alone | P1 | Small |
| 9 | Empty states with one action | P1 | Medium |
| 10 | Merge muted and subtle text colours | P2 | Small |

## 13. Not measured

- Dark theme contrast and the accent variants.
- Focus rings, disabled and error states.
- The folder-choose dialog, the file viewer and the other admin pages.
- Keyboard order and screen-reader output.
- Widths below 56 rem, except where noted.
