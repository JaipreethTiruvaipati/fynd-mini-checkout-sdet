# Test Plan: Mini Checkout

## 1. Scope

**In scope:** the checkout page (`internal/web/static`) and the Go API it uses (`GET /api/products`, `POST /api/orders`). Covers adding and removing products, quantities, line and order totals, the required name field, validation messages, placing an order, the confirmation and reference, and duplicate-order protection.

**Out of scope:** real payments, persistence (orders are in memory), auth, email, inventory. Order processing is simulated with a configurable delay (`-order-delay`, 600 ms by default; 300 ms in the browser tests) so that double-submission is a real race, not a theoretical one.

## 2. Test data (all synthetic)

| Product | Unit price | Why this value |
|---|---|---|
| A5 Dotted Notebook | ₹149.75 | Non-round. `3 × 149.75` in floating point is `449.24999…`, so it catches float maths |
| LED Desk Lamp | ₹1,299.00 | Above ₹1,000, so it checks the thousands separator |

Customer name `Asha Verma`. Quantity range 1–10 per line.

## 3. Risks, in priority order

| # | Risk | Customer impact | Likelihood | Priority | Covered by |
|---|---|---|---|---|---|
| R1 | **Duplicate order**: double-click, repeated Enter, or a retry after a lost response | Charged twice | High | **P1** | EC-03, EC-04, EC-05, **EC-06**, unit: concurrent duplicates, MT-02 |
| R2 | **Wrong amount**: float rounding, stale total, removed item still counted, UI total ≠ charged total | Charged the wrong amount | Medium | **P1** | TC-03, TC-01 (UI vs server), unit: pricing, `price_changed` |
| R3 | **Order with invalid data**: blank or whitespace name, empty cart, invalid quantity | Unfulfillable or wrong order | Medium | **P1** | TC-02, EC-01, EC-02, EC-07, EC-08, MT-01, unit: validation table |
| R4 | **Click lost or ignored in some browsers** (only a cross-browser run shows this) | "Nothing happens", customer leaves | Medium | **P1** | EC-08 on 4 browser profiles (DEF-02) |
| R5 | Unclear or missing confirmation | Customer re-orders or calls support | Low | P2 | TC-01, EC-03 |
| R6 | Input lost after a validation error | Frustration, abandoned cart | Medium | P2 | TC-02, EC-01, EC-07 |
| R7 | Layout breaks on phones; controls off-screen | Can't edit cart on mobile | Medium | P2 | EC-10 (DEF-03) |
| R8 | State leaks between orders | Old items in a new order | Low | P3 | EC-09 |
| R9 | API abuse: malformed JSON, unknown fields, client-supplied prices, wrong method | Wrong orders, errors | Low | P3 | unit: HTTP API tests |

## 4. Approach

**Test pyramid**
1. **Unit tests (Go, `go test -race`)**: pricing, Indian currency formatting, table-driven validation, idempotency (including 50 concurrent identical requests), HTTP status codes. Fast and exact.
2. **Browser tests (Go + playwright-go)**: the customer journeys in **Chromium, Firefox, WebKit and a mobile Chromium profile (Pixel 7)**.
3. **Manual and exploratory testing** in Google Chrome: empty cart and rapid double-click, as the brief asks, plus exploratory testing around quantity input.

**Browser test rules**
- **Clean state:** every test gets a new Go server (empty order store) and a new browser context, and asserts the empty starting state before doing anything.
- **Assert what the customer sees:** role/label locators (`GetByRole`, `GetByLabel`) and exact ₹ text. Every action is followed by an assertion.
- **Also assert the source of truth:** tests read the server's order store to prove "exactly one order" and "charged exactly the amount shown".
- **No fixed waits:** only web-first assertions that retry until true (5 s timeout).
- **No retries.** A flaky test is investigated, not hidden (see FLAKE-01 in the results).
- **Failure diagnostics:** on failure the harness saves a full-page screenshot plus a timestamped log of requests, responses, console messages and JS errors.

**Checking the tests themselves (mutation check):** bugs are injected on purpose to confirm the suite fails. See TEST_RESULTS §4.

## 5. Test cases

| ID | Level | Case | Expected |
|---|---|---|---|
| **TC-01** | Browser | Add both products, enter name, place order | One "Order confirmed", reference `ORD-XXXXXX`, ₹1,448.75. Server holds exactly that order and amount |
| **TC-02** | Browser | Blank / whitespace-only name | "Please enter your full name.", field `aria-invalid` and focused, cart kept, **no request sent**. After fixing, the order succeeds |
| **TC-03** | Browser | Qty 3 → ₹449.25; + lamp → ₹1,748.25; + → ₹1,898.00; − → ₹1,748.25; remove lamp → ₹449.25; place order | Exact totals at each step. Server charges 44925 paise for qty 3 |
| EC-01 | Browser | Empty cart, name entered | Blocked, message, name kept, no request |
| EC-02 | Browser | Empty cart and blank name | Both messages |
| EC-03 | Browser | Double-click Place order | 1 request, 1 order, 1 confirmation |
| EC-04 | Browser | 3 × `requestSubmit()` in the same tick | 1 request, 1 order |
| EC-05 | Browser | Enter ×3 in the name field | 1 order |
| **EC-06** | Browser | **Order created but response lost** (request intercepted and aborted after reaching the server), then customer clicks again | Network message shown, then retry → **still 1 order**, and the customer sees its reference |
| EC-07 | Browser | Qty `0`, `11`, `2.5`, `-1`, blank, `abc`, `1e1` | Message, input kept, totals show "—", checkout blocked, no request. Valid qty clears it |
| EC-08 | Browser | Type a qty and click Place order immediately (no blur) | Valid → charged for the typed qty. Invalid → blocked with message (DEF-01, DEF-02) |
| EC-09 | Browser | Start a new order | Clean state; next order gets a new reference |
| EC-10 | Browser | Two cart lines on each viewport | No sideways scroll; Remove on-screen and works (DEF-03) |
| MT-01 | Manual | Empty cart → Place order | Blocked, clear message, name kept |
| MT-02 | Manual | Rapid double-click | One confirmation; reference stable |
| Unit | Go | Formatting, pricing, 10 validation rules, boundaries, idempotency, concurrency, cancel/retry, HTTP codes | See `internal/**/_test.go` |

## 6. Entry and exit criteria

- **Entry:** `go build ./...` succeeds and browsers are installed.
- **Exit:** all unit and browser tests pass on all four profiles with 0 retries, a repeat run shows no new flakiness, both manual tests are recorded, and every defect found is reported with evidence.

## 7. Not tested / known gaps

- Real payment provider, persistence across restarts, multiple server instances (the idempotency store is in memory; production would use a shared store such as Redis or a DB unique key).
- No full accessibility audit (axe or a screen reader). Only `aria-invalid`, focus, labels and live regions are checked.
- No visual-regression or load testing.
- Real Safari and real phones were not tested. WebKit and the Pixel 7 profile are the closest stand-ins.
- One intermittent Firefox failure (FLAKE-01) is still under investigation.
