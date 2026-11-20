# Test Results — Mini Checkout

**Environment:** macOS (Darwin 25), Node v24.14.1, Playwright 1.65.1 (Chromium headless shell), Google Chrome for manual tests. App served by `node server.js` at `http://localhost:4173`.
**Date:** 20 Nov 2026.

## 1. Summary

| | Result |
|---|---|
| Automated tests | **12 / 12 passed** (0 retries) — raw output in [`test-run-output.txt`](test-run-output.txt) |
| Flakiness check | `--repeat-each=10` → 110/110 passed (before EC-08 was added); `--repeat-each=5` → 60/60 passed (after) |
| Manual tests | MT-01 **Pass**, MT-02 **Pass** |
| Defects found | **1 (DEF-01, Major)**, found during exploratory testing. **Fixed** and covered by regression test EC-08 |

## 2. Automated results

```
✓ TC-01: a successful checkout shows exactly one confirmation with a reference
✓ TC-02: a blank name blocks checkout, shows a clear message and keeps the cart
✓ TC-02: a whitespace-only name blocks checkout, shows a clear message and keeps the cart
✓ TC-03: changing quantity updates line totals and the exact order total
✓ EC-01: an empty cart blocks checkout and keeps the entered name
✓ EC-02: an empty cart and blank name show both messages
✓ EC-03: a rapid double-click on Place order creates one order
✓ EC-04: two submits fired in the same tick still create one order
✓ EC-05: pressing Enter repeatedly in the name field creates one order
✓ EC-06: invalid quantities are rejected and the last valid quantity is kept
✓ EC-07: "Start a new order" returns to a clean state
✓ EC-08 (DEF-01 regression): an invalid quantity typed just before Place order blocks checkout
12 passed
```

Reproduce with `npm test`. Open the HTML report with `npm run report`.

## 3. Manual test results

### MT-01 — Empty cart

| | |
|---|---|
| Steps | 1. Open `http://localhost:4173` (fresh load). 2. Leave the cart empty. 3. Type `Asha Verma` in Full name. 4. Click **Place order**. |
| Expected | Checkout blocked; clear message; name kept; no confirmation. |
| Actual | Message *"Your cart is empty. Add at least one item before placing your order."* shown under the name field. The name `Asha Verma` was still there. No confirmation appeared. Adding an item then cleared the message. |
| Result | **Pass** |
| Evidence | [`evidence/MT-01-empty-cart-blocked.jpg`](evidence/MT-01-empty-cart-blocked.jpg) |

### MT-02 — Rapid double-click on Place order

| | |
|---|---|
| Steps | 1. Fresh load. 2. Add *A5 Dotted Notebook* (total ₹149.75). 3. Enter `Asha Verma`. 4. Double-click **Place order** quickly. 5. Wait about 2 s and check the page again. |
| Expected | Exactly one confirmation; reference does not change; one order created. |
| Actual | One "Order confirmed" panel with reference **ORD-JMFZHP** and amount ₹149.75. Two seconds later the reference was the same, there was still one confirmation, and the page's order counter (`body[data-orders-placed]`) was `1`. The Place order button is no longer available after confirmation. |
| Result | **Pass** |
| Evidence | [`evidence/MT-02-double-click-single-confirmation.jpg`](evidence/MT-02-double-click-single-confirmation.jpg), [`evidence/MT-02-double-click-after-2s.jpg`](evidence/MT-02-double-click-after-2s.jpg) |
| Note | The manual screenshot came after the 600 ms processing window, so it doesn't show the intermediate disabled *"Placing order…"* state. That state is asserted in automated test EC-03. |

## 4. Does the suite catch real bugs? (mutation check)

I broke the app on purpose three times, ran the suite, then restored the code:

| Bug injected | Tests that failed |
|---|---|
| Removed the submit guard and stopped disabling the button | EC-03, EC-04, EC-05 (3 failed) |
| Name no longer required | TC-02 (blank), TC-02 (whitespace), EC-02 (3 failed) |
| Line totals miscalculated by 0.1% | TC-01, TC-02 ×2, TC-03, EC-06, EC-08 (6 failed, all of which assert exact totals) |

## 5. Defect report

### DEF-01 — Order is placed when an invalid quantity is entered right before clicking Place order

| Field | Detail |
|---|---|
| Severity / Priority | **Major / P1.** The customer sees a validation error and an order confirmation at the same time, and the order uses a quantity they didn't confirm. |
| Status | **Fixed** (verified manually and by regression test EC-08) |
| Found by | Exploratory testing in Chrome around quantity input |
| Build | Before fix (initial build) |
| Steps to reproduce | 1. Fresh load. 2. Add *A5 Dotted Notebook*. 3. Enter `Asha Verma`. 4. Select the quantity field and type `0` (don't press Tab or Enter). 5. Click **Place order** straight away. |
| Expected | Checkout is blocked until the quantity is fixed. No order is created. |
| Actual | The quantity error *"Enter a whole number from 1 to 10."* appeared **and** an order was confirmed (**ORD-MDGCDH**, ₹149.75 for quantity 1) at the same moment. |
| Root cause | Clicking the button blurs the field and fires `change`. `setQty` rejects `0` and keeps the old quantity (1), then `submit` runs. `validate()` checked only the cart and name, not rejected quantity edits. |
| Fix | `validate()` also blocks checkout while any quantity error is showing: *"Fix the highlighted quantity before placing your order."* (`app/app.js`, `validate()`). |
| Evidence | Before: [`evidence/DEF-01-invalid-qty-order-still-placed.jpg`](evidence/DEF-01-invalid-qty-order-still-placed.jpg) · After fix: [`evidence/DEF-01-after-fix-blocked.jpg`](evidence/DEF-01-after-fix-blocked.jpg) |
| Regression test | `EC-08` in `tests/edge-cases.spec.js` failed before the fix (`Expected: "Fix the highlighted quantity…" Received: ""`) and passes after. |

## 6. Observations (not defects)

- **Duplicate-order protection is client-side only.** It holds against double-click, repeated Enter and same-tick submits. A real store would also need a server-side idempotency key, because a second tab or a retried network request would bypass the browser guard.
- Typing `1e1` in the quantity field is read as `10` (the browser's number input accepts exponent notation). The result is valid and the total is correct, so it isn't logged as a defect.
