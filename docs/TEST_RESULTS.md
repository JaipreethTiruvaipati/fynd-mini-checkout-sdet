# Test Results: Mini Checkout

**Environment:** macOS 26 (Apple Silicon), Go 1.26.1, playwright-go v0.6401.0 (Chromium, Firefox 159, WebKit 27.2, Pixel 7 profile), Google Chrome for manual tests.
**Build:** Go version (v2). Defects DEF-01 to DEF-03 were found on the earlier builds and are fixed in this one.
**Date:** 21 Nov 2026.

## 1. Summary

| | Result |
|---|---|
| Unit tests (`go test -race`) | **28 / 28 passed** |
| Browser tests (15 scenarios × 4 browser profiles) | **60 / 60 passed**, retries off |
| HTML report | [`report/index.html`](report/index.html), generated from `go test -json` by `cmd/testreport` |
| Manual tests | MT-01 **Pass**, MT-02 **Pass** |
| Defects found | **3**: DEF-01 Major, DEF-02 Major, DEF-03 Minor. **All fixed**, each with a regression test |
| Flaky tests | **1 open**: FLAKE-01 (Firefox only, 2 failures in 39 Firefox suite runs) |

## 2. Browser test results

| Scenario | Chromium | Firefox | WebKit | Mobile |
|---|:-:|:-:|:-:|:-:|
| TC-01 Successful checkout → one confirmation | ✔ | ✔ | ✔ | ✔ |
| TC-02 Missing name blocks checkout (blank) | ✔ | ✔ | ✔ | ✔ |
| TC-02 Missing name blocks checkout (whitespace) | ✔ | ✔ | ✔ | ✔ |
| TC-03 Quantity change → exact total | ✔ | ✔ | ✔ | ✔ |
| EC-01 Empty cart blocked, name kept | ✔ | ✔ | ✔ | ✔ |
| EC-02 Empty cart + blank name → both messages | ✔ | ✔ | ✔ | ✔ |
| EC-03 Double-click → 1 request, 1 order | ✔ | ✔ | ✔ | ✔ |
| EC-04 3 submits in one tick → 1 request | ✔ | ✔ | ✔ | ✔ |
| EC-05 Enter ×3 → 1 order | ✔ | ✔ | ✔ | ✔ |
| EC-06 Lost response + retry → still 1 order | ✔ | ✔ | ✔ | ✔ |
| EC-07 Invalid quantities rejected | ✔ | ✔ | ✔ | ✔ |
| EC-08 Type qty, click immediately (valid) | ✔ | ✔ | ✔ | ✔ |
| EC-08 Type qty, click immediately (invalid) | ✔ | ✔ | ✔ | ✔ |
| EC-09 Start a new order resets | ✔ | ✔ | ✔ | ✔ |
| EC-10 Cart fits the screen | ✔ | ✔ | ✔ | ✔ |

Reproduce: `go test -tags e2e ./e2e -v`. Single engine: `BROWSERS=webkit go test -tags e2e ./e2e -v`.

## 3. Manual tests (Google Chrome, Go build)

### MT-01: Empty cart
| | |
|---|---|
| Steps | 1. `go run ./cmd/server`, open `http://127.0.0.1:8080`. 2. Leave the cart empty. 3. Type `Asha Verma`. 4. Click **Place order**. |
| Expected | Blocked; clear message; name kept; no order. |
| Actual | *"Your cart is empty. Add at least one item before placing your order."* shown under the name field. `Asha Verma` was still in the field. No confirmation, and the server log shows no order request. |
| Result | **Pass**. Evidence: [`evidence/MT-01-empty-cart-blocked.jpg`](evidence/MT-01-empty-cart-blocked.jpg) |

### MT-02: Rapid double-click on Place order
| | |
|---|---|
| Steps | Continue from MT-01: add *A5 Dotted Notebook* (₹149.75), then double-click **Place order** quickly. Wait 2 s. Check the page and the server log. |
| Expected | One confirmation, one order. |
| Actual | One "Order confirmed" panel, **ORD-YTJYVX**, ₹149.75, unchanged after 2 s. The server log has exactly **one** `order placed` line and no `duplicate submit` line, so the second click didn't even send a request. |
| Result | **Pass**. Evidence: [`evidence/MT-02-double-click-one-confirmation.jpg`](evidence/MT-02-double-click-one-confirmation.jpg), [`evidence/MT-02-server-log.txt`](evidence/MT-02-server-log.txt) |

## 4. Do the tests catch real bugs? (mutation check)

Each bug was injected into the code, the suite was run, and the code was restored:

| Injected bug | Caught by | What it shows |
|---|---|---|
| **M1** Remove the browser's "already submitting" guard | EC-04 (browser) | The server still deduplicated (1 order), but the test also checks **requests sent**, so it catches the regression before it reaches customers |
| **M2** Server ignores idempotency keys | 4 unit tests (incl. 50-goroutine concurrency test) + EC-06 lost-response test | The server is a real second line of defence, and it is tested |
| **M3** Browser stops checking the name | TC-02 (both variants), EC-02 | Asserting "no request sent" catches it even though the server would also reject the order |

## 5. Defect reports

### DEF-01: Order placed while a quantity error is on screen (Major, fixed)
| Field | Detail |
|---|---|
| Found in | Build v1 (vanilla JS), exploratory testing in Chrome |
| Steps | 1. Add *A5 Dotted Notebook*. 2. Enter `Asha Verma`. 3. Type `0` in the quantity box (don't press Tab). 4. Click **Place order** at once. |
| Expected | Checkout blocked until the quantity is fixed. |
| Actual | The quantity error appeared **and** an order was confirmed (ORD-MDGCDH, ₹149.75, qty 1) at the same moment. |
| Root cause | The click blurred the field. `change` rejected `0` and kept qty 1, then submit ran; validation didn't consider rejected quantity edits. |
| Fix | Checkout is blocked while any quantity is invalid ("Fix the highlighted quantity before placing your order."). Server-side validation also rejects qty outside 1–10. |
| Evidence | [before](evidence/DEF-01-invalid-qty-order-still-placed.jpg) · [after](evidence/DEF-01-after-fix-blocked.jpg) |
| Regression test | EC-08 "invalid quantity blocks" (all browsers), EC-07 |

### DEF-02: First click on Place order ignored in Firefox and WebKit after editing a quantity (Major, fixed)
| Field | Detail |
|---|---|
| Found in | Build v1, when the suite was first run on Firefox and WebKit. Chromium was unaffected, so a Chrome-only suite would never have found it |
| Steps | 1. Add *A5 Dotted Notebook*. 2. Enter a name. 3. Type `3` (or `0`) in the quantity box. 4. Click **Place order** once. |
| Expected | Typed `3` → order for ₹449.25. Typed `0` → blocked with a message. |
| Actual | **WebKit:** with `3`, nothing happened (no order, no message, button idle); with `0`, nothing happened either. **Firefox:** with `0`, nothing happened. The customer gets no feedback at all. |
| Measured | Typed 3 → Chromium ✔ ₹449.25 · Firefox ✔ · WebKit ✘ ignored. Typed 0 → Chromium ✔ blocked · Firefox ✘ ignored · WebKit ✘ ignored |
| Root cause (most likely) | The quantity was applied on `change`, which fires when the field loses focus, i.e. in the middle of the click on Place order. The handler then **rebuilt the whole cart DOM** (and for `0` added an error line that pushed the button down). Firefox and WebKit didn't deliver that click to the button. Confirmed indirectly: after removing the mid-click DOM rebuild, EC-08 passes on every engine. |
| Fix | Quantity applies on every keystroke (`input`). Cart rows are created once and **updated in place**. Space for the error message is reserved, so nothing moves under the pointer. |
| Evidence | [WebKit, typed 3, click ignored](evidence/DEF-02-before-fix-webkit-qty3-click-ignored.png) · [Chromium qty 0](evidence/DEF-02-before-fix-chromium-qty0.png) · [Firefox qty 0](evidence/DEF-02-before-fix-firefox-qty0.png) · [WebKit qty 0](evidence/DEF-02-before-fix-webkit-qty0.png) |
| Regression test | EC-08 (both variants) on Chromium, Firefox, WebKit and mobile |

### DEF-03: Cart overflows on phones; Remove button off-screen (Minor, fixed)
| Field | Detail |
|---|---|
| Found in | Build v1, first run on the Pixel 7 profile (412 px wide) |
| Steps | Open on a 412 px-wide screen, add both products. |
| Expected | Everything fits; Remove can be tapped. |
| Actual | The 5-column table made the page 509 px wide. Remove sat at x=420–505, outside the 412 px viewport, and the test's tap timed out because other cells covered it. |
| Fix | Below 560 px each cart line is shown as a stacked card with labels. |
| Evidence | [before](evidence/DEF-03-before-fix-mobile.png) · [after](screenshots/07-mobile.png) |
| Regression test | EC-10 (checks no sideways scroll, Remove in viewport and working) on all 4 profiles |

## 6. Open issue

### FLAKE-01: Intermittent Firefox failure (open, not hidden by retries)
- **Seen:** 2 failures in 39 Firefox suite runs (EC-09 once, TC-01 once). **0** failures in Chromium, WebKit and mobile across the same runs.
- **Symptoms differ.** EC-09: the page stayed on "Placing order…" for more than 5 s although the server delay is 300 ms. TC-01: the server created the order and the confirmation was on screen in the failure screenshot, yet the visibility assertion failed after about 1.8 s.
- **Evidence:** [EC-09 screenshot](evidence/FLAKE-01-firefox-stuck-placing-order.png), [TC-01 screenshot](evidence/FLAKE-01-firefox-confirmation-rendered-but-assert-failed.png)
- **What I did:** added a timestamped browser event log (requests, responses, failures, console, JS errors) and server counters to every failure report, then stress-ran the suite: 23 runs in a row passed after that, so the root cause isn't confirmed yet.
- **Current hypotheses:** (a) a Firefox/Juggler timing issue in the test tooling (the TC-01 symptom points this way); (b) a delayed fetch response in Firefox (the EC-09 symptom). Next step: capture a Playwright trace in CI when it recurs.
- **Why not add retries:** a retry would make the dashboard green and hide a possible real "button stuck" bug from customers.

## 7. Observations (not defects)

- Duplicate protection now works at two layers: the browser (guard + disabled button) and the server (idempotency key). The server store is in memory; production would need a shared store such as a DB unique key or Redis `SETNX`.
- The server never trusts prices from the browser. It re-prices from its catalogue and refuses the order (`409 price_changed`) if the total differs from what the customer saw.
