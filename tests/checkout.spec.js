// The three required automated checks from the brief.
const { test, expect } = require('@playwright/test');
const { openCleanCheckout, ui, REFERENCE_PATTERN } = require('./helpers');

test.beforeEach(async ({ page }) => {
  // Playwright gives every test a brand-new browser context (no shared cookies/storage),
  // and we additionally assert the visible clean state before acting.
  await openCleanCheckout(page);
});

test('TC-01: a successful checkout shows exactly one confirmation with a reference', async ({ page }) => {
  const c = ui(page);
  await c.addNotebook();
  await c.addLamp();
  await expect(c.orderTotal).toHaveText('₹1,448.75');

  await c.name.fill('Asha Verma');
  await c.placeOrder.click();

  await expect(c.confirmationHeading).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Order confirmed' })).toHaveCount(1);
  await expect(page.getByText('Thank you, Asha Verma.')).toBeVisible();
  await expect(c.orderReference).toHaveText(REFERENCE_PATTERN);
  await expect(c.confirmTotal).toHaveText('₹1,448.75');
  // The checkout form is gone, so the customer cannot submit again.
  await expect(c.placeOrder).toBeHidden();
});

for (const [label, value] of [['blank', ''], ['whitespace-only', '   ']]) {
  test(`TC-02: a ${label} name blocks checkout, shows a clear message and keeps the cart`, async ({ page }) => {
    const c = ui(page);
    await c.addNotebook();
    await c.name.fill(value);
    await c.placeOrder.click();

    await expect(c.formError).toHaveText('Please enter your full name.');
    await expect(c.name).toHaveAttribute('aria-invalid', 'true');
    await expect(c.name).toBeFocused();
    await expect(c.confirmationHeading).toBeHidden();
    // What the customer already chose is preserved.
    await expect(page.getByTestId('cart-row-notebook')).toBeVisible();
    await expect(c.orderTotal).toHaveText('₹149.75');

    // Fixing the field clears the message and checkout then succeeds.
    await c.name.fill('Asha Verma');
    await expect(c.formError).toHaveText('');
    await c.placeOrder.click();
    await expect(c.confirmationHeading).toBeVisible();
  });
}

test('TC-03: changing quantity updates line totals and the exact order total', async ({ page }) => {
  const c = ui(page);
  await c.addNotebook();
  await expect(c.lineTotal('notebook')).toHaveText('₹149.75');

  // Type a quantity (decimal price chosen on purpose: 3 x 149.75 must be exactly 449.25).
  await c.qtyInput('A5 Dotted Notebook').fill('3');
  await c.qtyInput('A5 Dotted Notebook').press('Tab');
  await expect(c.lineTotal('notebook')).toHaveText('₹449.25');
  await expect(c.orderTotal).toHaveText('₹449.25');

  // A second product is added to the total.
  await c.addLamp();
  await expect(c.lineTotal('lamp')).toHaveText('₹1,299.00');
  await expect(c.orderTotal).toHaveText('₹1,748.25');

  // The +/- buttons update the same way.
  await page.getByRole('button', { name: 'Increase quantity of A5 Dotted Notebook' }).click();
  await expect(c.qtyInput('A5 Dotted Notebook')).toHaveValue('4');
  await expect(c.lineTotal('notebook')).toHaveText('₹599.00');
  await expect(c.orderTotal).toHaveText('₹1,898.00');

  await page.getByRole('button', { name: 'Decrease quantity of A5 Dotted Notebook' }).click();
  await expect(c.orderTotal).toHaveText('₹1,748.25');

  // Removing an item takes it out of the total.
  await page.getByRole('button', { name: 'Remove LED Desk Lamp' }).click();
  await expect(c.lineTotal('lamp')).toHaveCount(0);
  await expect(c.orderTotal).toHaveText('₹449.25');
});
