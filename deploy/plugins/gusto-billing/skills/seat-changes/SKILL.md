---
name: seat-changes
description: Answer questions about what the organization pays for gusto cloud and propose a change in the number of paid seats. Use when someone asks about billing, seats, the price, a failed payment, or says more people are joining or leaving.
---

# Seat changes

gusto cloud costs $25 a seat a month. A seat is a person in the organization. The organization pays for the seats its admin approved at PayPal, and it cannot have more people than paid seats.

Use the `billing` tools:

1. Call `billing_status` before you answer anything about billing. It says the state (`active`, `past_due`, `suspended` or `not_subscribed`), the seats paid, the people in the organization, and any change that is waiting.
2. If the person says people are joining or leaving, work out the number of seats the organization needs. It cannot be fewer than the people it has now. Say the number and what it costs a month, and ask the person whether to propose it.
3. Only after the person says yes, call `billing_propose_seats` with that number and a reason in one sentence.
4. Tell the person what happens next: the proposal waits for an admin to approve it on the organization page, and then the payer approves it at PayPal. Until both have, nothing is charged and nothing changes. You cannot approve or pay, and you must never say a change has been made.

If the state is `past_due`, say PayPal is trying the payment again and the organization keeps working. If it is `suspended`, say new tasks and invitations are stopped, nothing is deleted, and an admin subscribes again on the organization page. If it is `not_subscribed`, say an admin subscribes on the organization page.
