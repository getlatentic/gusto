# Design principles

## The first rule: simple on top, powerful underneath

A person sees one simple thing. The server does the hard work behind it.

- **One action to start.** Open the app, say what you want, send. The agent, model, sandbox, folder and keys have defaults, so nothing is chosen before a person can begin.
- **Choices appear when they matter.** Engine, model and preset are behind a menu with a good default. Settings for an organisation live in the admin pages, not in a person's way.
- **The person's words.** Never "harness", "proxy", "sandbox driver" or "cell". Say "your computer", "what it changed", "what it cost".
- **The same everywhere.** The computer can be the person's laptop or in the cloud, and the front end can be the app, the terminal or the browser. A person does the same thing in each.
- **Power stays reachable.** An admin, or a person who looks for it, can reach every control: engines, models, policy, records, spend.
- **Failures speak the person's language too.** When the machinery fails, the screen says what happened and what to do, not how the machinery works.

The test for a feature: a person using it for the first time needs no new word, and an admin can still reach all of its control. If the power has to show on the surface to work, the default is wrong.

The rules below make this checkable. Every rule has a check. A check is one of three kinds:

- **Auto**: `scripts/design-audit.js` or a browser test measures it.
- **Walk**: a reviewer does it in the running app (keyboard, zoom, stop an agent, switch theme).
- **Look**: a reviewer judges a screenshot and a second reviewer can agree.

## Gates

| Gate | What it covers | What a miss does |
|---|---|---|
| 1. Mandatory | Part 1 (trust) and Part 2 (accessibility) | Blocks release. |
| 2. Review | First-use comprehension, hierarchy, density, copy. A person who has not seen the screen says what it is for and what they would click first. | A reviewer decides. |
| 3. Consistency | Part 3 (the visual system) | A warning. A new value needs a written reason. It does not block release. |

## Order of precedence

When two rules conflict, the earlier one wins.

1. Security, permission, privacy and accessibility.
2. The person can see what the agent did, what it can reach, and how to stop it.
3. The person understands the screen and can recover from a mistake.
4. The person's settings apply (theme, accent, text size).
5. The screen uses the shared components.
6. The screen is clean.

Clean never means unlabelled. If removing an element makes a person lose their place, keep it.
Do not add a precedence item to settle one argument. Fix the rules that conflict.

## Words

Use one word for one idea. Define it where it first appears. Test it with a person who is not technical.

| Say | Not | Meaning |
|---|---|---|
| preset | role, template, harness | A starting point for a chat: what it is good at. |
| agent | bot | The AI that does the work in a chat. |
| engine, model | harness, provider | The program that runs the agent, and the model it uses. Chosen in the chat. |
| project | repo, instance | A folder an agent works in. |
| record | job, run | What one turn did, with its evidence. |
| activity | jobs | The log of records. |

## Part 1. Trust (gate 1)

| # | Rule | Check |
|---|---|---|
| 1 | **A running agent shows its true state.** States are queued, running, waiting for approval, stopping, stopped, failed, completed with errors, and done. The state is in words. Elapsed time is shown. A Stop control is shown. Stopping new actions is not the same as undoing work already done, and the screen says which one it did. One state model drives the chat, the activity log and every notification. | Walk: start, approve, stop, fail and retry a run. Every view agrees. |
| 2 | **A consequential action says what it will change, and asks first.** Deleting, sending, publishing, sharing and changing retention name the object, the destination and the effect. A permission prompt lists the files, commands or recipients. Undo is offered where it is possible. Where it is not, the screen says so before the action. Ask only for consequential actions, so people read the prompt. | Walk: the dialog names the item and the effect. Rejecting it changes nothing. |
| 3 | **AI output says what it is and where it came from.** It names the agent and model. It separates what was consulted, what was done, what was cited and what was checked. It never presents an unsupported claim as fact. It looks different from the person's text and from system text, and says "AI" in text, not only in style. | Walk: a screen reader reads who wrote each message. Compare the shown tools and files with the record. |
| 4 | **Everything an agent did is recorded.** The record shows who asked, who approved, which agent acted, what changed, when, and whether it worked. It cannot be edited. The person can export it. Who may see which entries is controlled, and a missing entry is shown as missing. | Walk: open an entry. You can say who authorised it and what changed. Reconcile it with what ran. |
| 5 | **An agent's access is visible.** The screen shows which files, tools and data an agent can reach, for how long, and how to take that back. | Look: every chat shows its access. |
| 6 | **An error says what failed, what it affected, and what to do.** It says when the cause is not known. It does not blame the person. It says whether the person's work is safe. It never shows a raw code alone. | Walk: cause an error. The text has a cause, an effect and one next step. |
| 7 | **Work is not lost.** Text being written is kept when a person leaves a page. A deleted thing can be brought back for a set time, and the screen says how long. Where it cannot, the screen says so first. | Walk: type, navigate away, return. |
| 8 | **Content made by someone else keeps its own look.** A web page or file made by another person or by an agent is shown in an isolated frame (white, light scheme) with a label that says who made it. The app's theme never restyles it. A way to read it as plain text is offered. | Walk: switch to dark. The app changes and the page does not. The frame is labelled. |
| 9 | **Say where data goes.** The screen says which model sees the person's data, where it is processed, and how long it is kept, in plain words. | Look: the information is one click from any chat. |
| 10 | **Deployment context is visible when it matters.** A classification banner, the signed-in role, and a session-timeout warning appear where the organisation turns them on. Timestamps show the time zone. | Walk: turn each on and check it. |

## Part 2. Accessibility (gate 1)

The target is WCAG 2.2 level AA. Where we choose a stricter number, the rule says so.

| # | Rule | Check |
|---|---|---|
| 11 | **Text contrast is 4.5:1.** Large text (24px, or 18.66px bold) is 3:1. Contrast is met in every theme and every accent, and in every state (hover, focus, disabled, selected). | Auto: contrast on every text and surface pair. |
| 12 | **Non-text contrast is 3:1.** This covers the borders of inputs, icons that carry meaning, status marks and focus rings. | Auto and Look. |
| 13 | **Focus is always visible and never hidden.** The focus ring is 2px with 3:1 against both sides. A sticky bar does not cover it. | Walk: tab through every control. |
| 14 | **Everything works by keyboard.** Order follows reading order. Nothing traps focus. A dragged control has a keyboard alternative. A tooltip opens on focus as well as hover, and carries nothing the label does not. | Walk. |
| 15 | **A target is at least 24px. Our default is 36px, and 44px on touch.** Spacing may stand in for size only as WCAG 2.5.8 allows. | Auto: bounding box of every control. |
| 16 | **Controls have a name, a role and a state.** A real element is used, not a styled box. Selected, expanded and busy are exposed. The visible label is part of the accessible name. | Auto (axe) and Walk with a screen reader. |
| 17 | **Changes are announced.** A streamed reply, a status change and a new error are announced politely and once. An error does not disappear on its own. | Walk with a screen reader. |
| 18 | **The page survives enlarged text.** At 200% text, and at 320px width, nothing is lost and nothing scrolls sideways (a table or a log may scroll). Overridden text spacing (line height 1.5, letter 0.12em, word 0.16em) loses nothing. | Walk. |
| 19 | **Motion is small and optional.** Durations are 320ms or less. Nothing moves under `prefers-reduced-motion`. Motion is never the only signal. | Auto and Walk. |
| 20 | **High contrast works.** The page is usable under forced colours. | Walk. |
| 21 | **Forms say what is wrong.** The label is above the field. The error is below it, in words, linked to the field. A summary is shown on submit. | Walk. |

## Part 3. The visual system (gate 3)

Values come from the tokens in `theme.css`. A value outside the scale is a warning. All sizes are written in rem in the CSS, so that they follow the person's text size, and `units_test.go` rejects px in the stylesheets. The pixel values in this document are what a rem is at the default 16px text, and are given so that the rules can be read and measured. The audit script reads computed pixels at 100% text.

| rem | 0.25 | 0.5 | 0.75 | 1 | 1.125 | 1.25 | 1.375 | 1.5 | 1.75 | 2 | 2.5 | 3 | 4 |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| px | 4 | 8 | 12 | 16 | 18 | 20 | 22 | 24 | 28 | 32 | 40 | 48 | 64 |

Space uses 0.25, 0.5, 0.75, 1, 1.25, 1.5, 2, 2.5, 3 and 4. Radius uses 0.25, 0.5, 0.75, 1, 1.25 and 1.5. Type uses 0.75, 0.8125, 0.875, 1, 1.125, 1.375 and 1.5.

### Layout

| Topic | Rule | Check |
|---|---|---|
| Columns | A page has one or two main columns. A dashboard may have more. Columns change with the width the page has, not with the device. | Look, and narrow the window. |
| Alignment | Blocks share a few left edges. A label and its value share a baseline. Numbers in a column align right. Only a hero line, an empty state or a dialog is centred. | Auto for centred rows. Look for the rest. |
| Space scale | 4, 8, 12, 16, 20, 24, 32, 40, 48, 64px. | Auto: padding and gap are on the scale. |
| Grouping | Related items sit 8 to 16 apart. Groups sit 24 to 32 apart. Sections sit 40 or more apart. Set `gap` on the parent. | Look: related things are closer than unrelated things. |
| Use the room | A component takes the width its container offers. Only prose and forms are capped. Tables, logs and dashboards fill the page. | Walk: widen the window. Nothing small stays beside empty space. |
| Line length | Prose is at most 68ch wide. | Auto. |

### Surfaces

| Topic | Rule | Check |
|---|---|---|
| Layers | Rail, side panel, page, card. Neighbouring layers differ by one step. Hierarchy and contrast decide, in both themes. | Auto: the count of large background colours (warn above 5). |
| Edges | A card has one edge: a border, or a shadow, or a background step. A menu, a dialog and the composer may have a border and a shadow. A focus ring, a selection mark, an error and a safety indicator are always allowed. | Auto: warn on border plus shadow outside those. |
| Border | 1px. `--yg-border` by default. `--yg-border-strong` for inputs and the selected item. A border separates two things or holds an input. Do not draw one where a gap or a background step already separates. Input borders pass 3:1. | Look: remove the border. If the layout still reads, it was decoration. |
| Nesting | At most two ancestors with a border or shadow. A row in a card is divided by a hairline, not boxed. | Auto. |
| Radius | 4 (small marks), 8 (inputs, buttons, chips), 12 (cards, menus), 16 (dialogs), 20 (the composer), 24 (the largest panels). Full round for avatars and the send button. A shape inside another has a smaller radius. | Auto: warn on other values. |
| Shadow | Light theme: `--yg-shadow-1` for a card, `--yg-shadow-3` for a menu or dialog. Dark theme: a lighter surface, no shadow on cards. | Auto: only the tokens. |

### Colour

| Topic | Rule | Check |
|---|---|---|
| Greys | Neutral, from the `theme.css` neutrals only. | Auto: no hex outside tokens. |
| Accent | One accent fill for the primary action of a decision context (a page, a dialog, a card), and one mark for the current place. Focus, selection and error indicators may use it too. | Look. |
| Status | Running is neutral and has a spinner and a word. Waiting for approval is amber. Failed is red. Done is green. Every status has a word and an icon. Its text passes 4.5:1. A diff may use green and red surfaces. | Auto: contrast. Look: greyscale. |

### Type

| Topic | Rule | Check |
|---|---|---|
| Families | Sora for headings, Noto Sans for text, Noto Sans Mono for code. | Auto. |
| Sizes | 12, 13, 14, 16, 18, 22, 24px. A page title is 22. A hero line is 24. Messages and prose are 14. Secondary text is 13. A label is 12 or 13. A section heading is 16 or 18. Nothing is under 12 at 100% text. | Auto: warn on other sizes. |
| Weights | 400 for text, 500 for labels and controls, 600 for headings and the primary action. Nothing is above 600. | Auto: warn on other weights. |
| Emphasis | One signal per piece of text: heavier, or larger, or coloured. A heading may combine two. In a list row, only the title is 600. | Look. |
| Line height | Text 1.5. Headings 1.2 to 1.3. Labels 1.3 to 1.4. | Auto. |
| Case | Sentence case. No all-caps text. | Auto: `text-transform` is not `uppercase`. |
| Wrapping | A label or button does not wrap at the default size. It may wrap at enlarged text or narrow width. A wrapped heading is balanced and has no one-word last line. A secondary identifier may be cut with an ellipsis if its full value is reachable by keyboard, by touch and by a screen reader. A name, an ID or a clinical term is never cut. | Auto and Walk. |
| Do not shrink | If text does not fit, grow the container, wrap the row, or shorten the text. Do not reduce the font size. | Walk: narrow the window. |

### Components

| Topic | Rule | Check |
|---|---|---|
| Hierarchy | A place that holds others is shown above them and set apart, not as one more item in the list it contains. | Look: the top of a tree is not a sibling of its children. |
| Clean | Remove decoration. Never remove a visible label, heading, group or instruction without replacing it. | Look: cover an element. If nothing is lost, delete it. |
| Reuse | One button, one table, one dialog, one menu. A new variant needs a written reason. | Look: size, radius and type match the shared ones. |
| Controls | One primary, one quiet and one destructive style. Hover, focus and disabled are all defined. The primary is the accent fill and a region (a card, a form, a dialog) has one: the action it is for. Everything else a person presses is quiet, including the actions on a row and every change that can be undone, such as turning something off. What cannot be undone is destructive: the quiet line in the danger colour, never filled, and not shown as the primary. Hover does not turn a quiet button red. | Auto: more than one accent fill in a region, and a destructive word on one. Walk. |
| Icons | One style. 16 or 18px beside text. 20 or 22px in the rail. Stroke 1.75. An icon that carries meaning has a visible label, or a name and a tooltip when space is short. | Look. |
| Density | Each region has one density. Chat is roomy. A table or a log is compact. | Look. |

## How we work

1. Draw the screen on the design canvas first.
2. Build it to match the drawing.
3. Check it in the real app: light and dark, text enlarged, narrow and wide, keyboard only.
4. Show it to a person who has not seen it. Ask what the page is for and what they would click first.
5. Run `scripts/design-audit.js` on the page. Fix the warnings, or write down the reason for each one.

## Measuring

`scripts/design-audit.js` runs in a page and returns one object: the radius values, font sizes, weights, background colours, border and shadow use, nesting, text under 12px, all-caps text, wrapped labels, small targets, siblings in a centred row that sit off the centre, text under contrast, a region with more than one primary button, and a destructive word on a primary. A value outside this document is listed under `over`. It does not replace a walk with a keyboard or a screen reader.
