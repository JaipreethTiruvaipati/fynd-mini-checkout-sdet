// Extra regression checks for the cases the brief asks to test manually
// (empty cart, rapid double-click). The manual results are in docs/TEST_RESULTS.md.
const { test, expect } = require('@playwright/test');
const { openCleanCheckout, ui, REFERENCE_PATTERN } = require('./helpers');

test('EC-01: an empty cart blocks checkout and keeps the entered name', async ({ page }) => {
  await openCleanCheckout(page);
  const c = ui(page);
  await c.name.fill('Asha Verma');
  await c.placeOrder.click();

  await expect(c.formError).toHaveText('Your cart is empty. Add at least one item before placing your order.');
  await expect(c.name).toHaveValue('Asha Verma');
  await expect(c.confirmationHeading).toBeHidden();
  await expect(page.locator('body')).toHaveAttribute('data-orders-placed', '0');
});

test('EC-02: an empty cart and blank name show both messages', async ({ page }) => {
  await openCleanCheckout(page);
  const c = ui(page);
  await c.placeOrder.click();
  await expect(c.formError).toContainText('Your cart is empty.');
  await expect(c.formError).toContainText('Please enter your full name.');
});

test.describe('double submission', () => {
  test.beforeEach(async ({ page }) => {
    // A fake clock lets us run the simulated processing delay to completion
    // deterministically instead of sleeping.
    await page.clock.install();
    await openCleanCheckout(page);
    const c = ui(page);
    await c.addNotebook();
    await c.name.fill('Asha Verma');
  });

  test('EC-03: a rapid double-click on Place order creates one order', async ({ page }) => {
    const c = ui(page);
    await c.placeOrder.dblclick();
    await expect(c.placeOrder).toBeDisabled();
    await expect(c.placeOrder).toHaveText('Placing order…');

    await page.clock.runFor(5000); // well past the processing delay
    await expect(c.confirmationHeading).toHaveCount(1);
    await expect(c.confirmationHeading).toBeVisible();
    await expect(page.locator('body')).toHaveAttribute('data-orders-placed', '1');
    await expect(c.orderReference).toHaveText(REFERENCE_PATTERN);
  });

  test('EC-04: two submits fired in the same tick still create one order', async ({ page }) => {
    // Bypasses the disabled-button protection to prove the guard in the submit handler.
    await page.evaluate(() => {
      const form = document.getElementById('checkout-form');
      form.requestSubmit();
      form.requestSubmit();
      form.requestSubmit();
    });
    await page.clock.runFor(5000);
    const reference = await ui(page).orderReference.textContent();
    await page.clock.runFor(5000);
    await expect(page.locator('body')).toHaveAttribute('data-orders-placed', '1');
    await expect(ui(page).orderReference).toHaveText(reference);
  });

  test('EC-05: pressing Enter repeatedly in the name field creates one order', async ({ page }) => {
    const c = ui(page);
    await c.name.press('Enter');
    await c.name.press('Enter');
    await c.name.press('Enter');
    await page.clock.runFor(5000);
    await expect(page.locator('body')).toHaveAttribute('data-orders-placed', '1');
  });
});

test('EC-06: invalid quantities are rejected and the last valid quantity is kept', async ({ page }) => {
  await openCleanCheckout(page);
  const c = ui(page);
  await c.addNotebook();
  const qty = c.qtyInput('A5 Dotted Notebook');
  for (const bad of ['0', '11', '2.5', '-1', '']) {
    await qty.fill(bad);
    await qty.press('Tab');
    await expect(page.getByRole('alert').filter({ hasText: 'Enter a whole number from 1 to 10.' })).toBeVisible();
    await expect(qty).toHaveValue('1');
    await expect(c.orderTotal).toHaveText('₹149.75');
  }
});

test('EC-07: "Start a new order" returns to a clean state', async ({ page }) => {
  await openCleanCheckout(page);
  const c = ui(page);
  await c.addLamp();
  await c.name.fill('Asha Verma');
  await c.placeOrder.click();
  await expect(c.confirmationHeading).toBeVisible();

  await page.getByRole('button', { name: 'Start a new order' }).click();
  await expect(page.getByText('Your cart is empty.')).toBeVisible();
  await expect(c.orderTotal).toHaveText('₹0.00');
  await expect(c.name).toHaveValue('');
  await expect(c.confirmationHeading).toBeHidden();
});

test('EC-08 (DEF-01 regression): an invalid quantity typed just before Place order blocks checkout', async ({ page }) => {
  await openCleanCheckout(page);
  const c = ui(page);
  await c.addNotebook();
  await c.name.fill('Asha Verma');
  await c.qtyInput('A5 Dotted Notebook').fill('0');
  // Click straight away, without leaving the field first — this is what customers do.
  await c.placeOrder.click();

  await expect(c.formError).toHaveText('Fix the highlighted quantity before placing your order.');
  await expect(c.confirmationHeading).toBeHidden();
  await expect(page.locator('body')).toHaveAttribute('data-orders-placed', '0');

  // Correcting the quantity lets checkout continue, with the corrected total.
  await c.qtyInput('A5 Dotted Notebook').fill('2');
  await c.qtyInput('A5 Dotted Notebook').press('Tab');
  await expect(c.formError).toHaveText('');
  await c.placeOrder.click();
  await expect(c.confirmationHeading).toBeVisible();
  await expect(c.confirmTotal).toHaveText('₹299.50');
});
