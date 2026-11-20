const { expect } = require('@playwright/test');

/** Opens the app and asserts the clean starting state every test relies on. */
async function openCleanCheckout(page) {
  await page.goto('/');
  await expect(page.getByText('Your cart is empty.')).toBeVisible();
  await expect(page.getByTestId('order-total')).toHaveText('₹0.00');
  await expect(page.getByLabel('Full name')).toHaveValue('');
  await expect(page.getByRole('heading', { name: 'Order confirmed' })).toBeHidden();
}

const ui = (page) => ({
  addNotebook: () => page.getByRole('button', { name: 'Add A5 Dotted Notebook to cart' }).click(),
  addLamp: () => page.getByRole('button', { name: 'Add LED Desk Lamp to cart' }).click(),
  name: page.getByLabel('Full name'),
  placeOrder: page.getByRole('button', { name: /place order|placing order/i }),
  formError: page.locator('#form-error'),
  orderTotal: page.getByTestId('order-total'),
  confirmationHeading: page.getByRole('heading', { name: 'Order confirmed' }),
  orderReference: page.getByTestId('order-reference'),
  confirmTotal: page.getByTestId('confirm-total'),
  lineTotal: (id) => page.getByTestId(`line-total-${id}`),
  qtyInput: (productName) => page.getByRole('spinbutton', { name: `Quantity of ${productName}` }),
});

const REFERENCE_PATTERN = /^ORD-[A-HJ-NP-Z2-9]{6}$/;

module.exports = { openCleanCheckout, ui, REFERENCE_PATTERN };
