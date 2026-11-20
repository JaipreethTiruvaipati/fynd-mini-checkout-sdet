# Test Plan — Mini Checkout

## 1. Scope

**In scope:** the single-page checkout in `app/`: adding and removing the two products, changing quantity, line and order totals, the required name field, validation messages, placing an order, the confirmation and its reference, and protection against duplicate orders.

**Out of scope (no such component exists):** payments, backend/API, persistence, auth, email. The app has no network calls. Order processing is simulated with a 600 ms delay so that double-submission is a real risk.

## 2. Test data (synthetic)

| Product | Unit price | Why |
|---|---|---|
| A5 Dotted Notebook | ₹149.75 | Non-round price. `3 × 149.75` exposes floating-point errors (`449.24999…`) |
| LED Desk Lamp | ₹1,299.00 | Above ₹1,000, so it checks the `en-IN` thousands separator |

Customer name: `Asha Verma` (synthetic). Quantity range is 1–10 per item.

## 3. Risks, in priority order

| # | Risk | Impact | Likelihood | Priority | Covered by |
|---|---|---|---|---|---|
| R1 | **Duplicate order** from a double-click, repeated Enter, or a slow response | Customer charged twice, support cost | High (real users double-click) | **P1** | MT-02, EC-03, EC-04, EC-05 |
| R2 | **Wrong total** (float rounding, stale total after a quantity change, removed item still counted) | Customer charged the wrong amount; trust/legal | Medium | **P1** | TC-03, TC-01 |
| R3 | **Order placed with invalid or missing data** (blank or whitespace name, empty cart, invalid quantity) | Unfulfillable or wrong order | Medium | **P1** | TC-02, MT-01, EC-01, EC-02, EC-08 |
| R4 | **No or unclear confirmation**: no reference, wrong amount, shown twice | Customer re-orders or contacts support | Low | P2 | TC-01, EC-03 |
| R5 | **Lost input after a validation error** (name or cart cleared) | Friction, abandoned checkout | Medium | P2 | TC-02, EC-01 |
| R6 | Invalid quantity input (0, negative, decimal, >10, blank) accepted | Wrong order or total | Medium | P2 | EC-06, EC-08 |
| R7 | State leaks between orders after "Start a new order" | Old items in a new order | Low | P3 | EC-07 |
| R8 | Accessibility: errors not announced, field not marked invalid | Some customers can't complete checkout | Medium | P3 | TC-02 (`aria-invalid`, focus, `role=alert`) — partial |

## 4. Approach

- **Automated browser tests (Playwright, Chromium)** for the three required checks (TC-01..03), plus regression checks for the P1 edge cases (EC-01..08).
  - **Clean state:** every test runs in a new browser context, and `openCleanCheckout()` first asserts the empty cart, ₹0.00 total, empty name and no confirmation.
  - **Assert what the customer sees:** locators by role and label (`getByRole`, `getByLabel`), exact text for money. Every action is followed by an assertion.
  - **No fixed waits:** web-first `expect` auto-retries. Double-submit tests use Playwright's fake clock (`page.clock`) to fast-forward past the processing delay deterministically.
  - **No retries** (`retries: 0`), so a flaky test shows up instead of being hidden.
- **Manual tests** in Google Chrome for the empty cart (MT-01) and rapid double-click (MT-02), as the brief asks, plus short exploratory testing around quantity input.
- **Suite effectiveness check:** I re-introduced three bugs by hand and confirmed the suite catches each one (see TEST_RESULTS.md §4).

## 5. Test cases

| ID | Type | Case | Expected |
|---|---|---|---|
| TC-01 | Auto | Add both products, enter name, place order | One "Order confirmed", reference `ORD-XXXXXX`, amount ₹1,448.75, form removed |
| TC-02 | Auto | Blank / whitespace-only name | "Please enter your full name.", field `aria-invalid` and focused, cart and total kept, no confirmation. After fixing, the order succeeds |
| TC-03 | Auto | Qty 3 → ₹449.25; add lamp → ₹1,748.25; + → ₹1,898.00; − → ₹1,748.25; remove lamp → ₹449.25 | Exact line and order totals at each step |
| MT-01 | Manual | Empty cart, name entered, Place order | Blocked with clear message, name kept, no order |
| MT-02 | Manual | Rapid double-click on Place order | Exactly one confirmation, reference doesn't change |
| EC-01/02 | Auto | Empty cart (± blank name) | Blocked, correct message(s), name kept |
| EC-03 | Auto | `dblclick()` on Place order | Button disabled ("Placing order…"), one order |
| EC-04 | Auto | Three `requestSubmit()` in the same tick (bypasses the disabled button) | One order, reference stable |
| EC-05 | Auto | Enter pressed three times in name field | One order |
| EC-06 | Auto | Qty `0`, `11`, `2.5`, `-1`, blank | Error shown, previous qty and total kept |
| EC-07 | Auto | Start a new order | Clean state restored |
| EC-08 | Auto | Type invalid qty and click Place order right away (DEF-01) | Checkout blocked |

## 6. Entry / exit criteria

- **Entry:** `npm install` and `npx playwright install chromium` succeed; the app loads at `http://localhost:4173`.
- **Exit:** all automated tests pass with 0 retries, a repeat run (`--repeat-each`) shows no flakiness, both manual tests are recorded, and any defects are reported with evidence.

## 7. Not tested / known gaps

- Only **Chromium** is automated. Firefox/WebKit and mobile viewports were not run (add them as Playwright projects).
- No full accessibility audit (axe, screen reader); only `aria-invalid`, focus and `role="alert"` are checked.
- No visual regression or responsive layout testing.
- Locale is fixed to `en-IN` / INR. Other locales are not supported.
- The duplicate-order guard is client-side only. A real system also needs a server-side idempotency key (see TEST_RESULTS.md, observations).
