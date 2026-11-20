# Mini Checkout & Browser Test Suite

Fynd Hiring Hackathon (Nov 2026), **SDET Intern** track.

A single-page checkout with two synthetic products, plus Playwright browser tests, a risk-based test plan, manual test results and a defect report.

| Deliverable | Location |
|---|---|
| Checkout app (HTML/CSS/vanilla JS, no backend) | [`app/`](app/) |
| Automated browser tests (Playwright) | [`tests/`](tests/) |
| Test plan (risks, priorities, cases, gaps) | [`docs/TEST_PLAN.md`](docs/TEST_PLAN.md) |
| Results, manual tests, defect report | [`docs/TEST_RESULTS.md`](docs/TEST_RESULTS.md) |
| Screenshots / evidence | [`docs/evidence/`](docs/evidence/) |

**Result:** 12/12 automated tests pass. Both manual tests (empty cart, rapid double-click) pass. **1 real defect found (DEF-01) and fixed**, with a regression test.

## Requirements

- Node.js 18 or newer (developed on v24.14.1)
- npm

## Run the app

```bash
npm install
npm start
# open http://localhost:4173
```

No build step. `server.js` is a small dependency-free static server for `app/`.

## Run the browser tests

```bash
npm install
npx playwright install chromium   # one-time browser download
npm test                          # headless; starts the app automatically
npm run test:headed               # watch it run in a real browser window
npm run report                    # open the HTML report from the last run
npx playwright test --repeat-each=10   # flakiness check
```

Playwright starts the app itself (`webServer` in `playwright.config.js`). If something is already running on port 4173 it is reused (outside CI).

## What the app does

- Two products with fixed prices: **A5 Dotted Notebook ₹149.75** and **LED Desk Lamp ₹1,299.00**.
- Add or remove items. Change quantity with −/+ or by typing (1–10). Line totals and the order total update immediately.
- Required **Full name** field. A blank or whitespace-only name, an empty cart, or an invalid quantity blocks checkout with a clear message. What the customer entered is kept.
- On success: one confirmation with a reference (`ORD-` + 6 characters, no look-alike 0/O/1/I) and the amount.
- **No duplicate orders:** the submit handler sets a `submitting` state synchronously before the simulated 600 ms processing step, so later submits are ignored. The button is also disabled ("Placing order…"), and the form is removed after confirmation.

### Design decisions

- **Money in integer paise.** Floats give `3 × 149.75 = 449.24999…`. Integer paise keep totals exact. The notebook's price was chosen to exercise this.
- **Simulated processing delay (600 ms).** Without any async step, double-submit would be trivially impossible and the test would prove nothing.
- **`body[data-orders-placed]`** is a small test hook that counts orders created on the page. Tests use it to prove "exactly one order", which the UI alone can't show if a duplicate replaced the first confirmation.

## Test approach (summary)

- **Required checks:** TC-01 successful checkout → confirmation; TC-02 missing name blocks checkout; TC-03 quantity change → exact total.
- **Extra regression checks:** empty cart, double-click, same-tick double submit, repeated Enter, invalid quantities, reset, DEF-01.
- Each test starts from a **clean state**: a new browser context, plus an assertion of the empty initial UI.
- Assertions check **what the customer sees** (role/label locators, exact ₹ text). **No fixed waits**: web-first assertions auto-retry, and double-submit tests use Playwright's **fake clock**.
- `retries: 0`, so flakiness can't hide. I checked that the suite catches bugs by injecting three on purpose (see TEST_RESULTS §4).

## Assumptions

- One currency (INR, `en-IN` formatting), quantity limited to 1–10 per item.
- No persistence: a page reload starts a new cart (also the clean state for tests).
- "Order placed" is simulated in the browser. There is no backend, payment or external service, as the brief allows.

## Known gaps / not tested

- Automated on **Chromium only**. Firefox, WebKit and mobile viewports were not run.
- No full accessibility audit (only `aria-invalid`, focus and `role="alert"` are checked).
- No visual regression testing.
- Duplicate-order protection is client-side only. A real backend would also need an idempotency key.

## Project structure

```
app/            index.html, app.js, styles.css
tests/          checkout.spec.js (required checks), edge-cases.spec.js, helpers.js
docs/           TEST_PLAN.md, TEST_RESULTS.md, test-run-output.txt, evidence/
server.js       static server
playwright.config.js
```

## AI assistance

This project was built with help from an AI coding assistant (Claude). I reviewed the code, tests and documents. All data is synthetic.
