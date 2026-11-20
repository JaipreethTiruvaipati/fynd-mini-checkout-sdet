'use strict';

/*
 * Mini Checkout UI.
 * - Prices come from GET /api/products. Money is kept in integer paise.
 * - The server (Go) re-prices and validates every order; the UI total is only a preview.
 * - Cart rows are created once and then updated in place. Rebuilding the DOM while the
 *   customer is clicking caused DEF-02 (the first click on "Place order" was lost).
 */

const MIN_QTY = 1;
const MAX_QTY = 10;
const MSG = {
  emptyCart: 'Your cart is empty. Add at least one item before placing your order.',
  nameMissing: 'Please enter your full name.',
  badQty: `Enter a whole number from ${MIN_QTY} to ${MAX_QTY}.`,
  fixQty: 'Fix the highlighted quantity before placing your order.',
  network: "We couldn't reach the store. Check your connection and try again.",
};

const inr = new Intl.NumberFormat('en-IN', { style: 'currency', currency: 'INR', minimumFractionDigits: 2 });
const formatMoney = (paise) => inr.format(paise / 100);

const state = {
  products: [],
  cart: new Map(),   // productId -> { qty, invalid } in insertion order
  status: 'idle',    // 'idle' | 'submitting' | 'placed'
  attemptKey: null,  // idempotency key for the current checkout attempt
  showErrors: false,
  serverMessage: '',
};

const $ = (id) => document.getElementById(id);
const els = {
  productsStatus: $('products-status'), productList: $('product-list'),
  cartEmpty: $('cart-empty'), cartTable: $('cart-table'), cartBody: $('cart-body'), orderTotal: $('order-total'),
  checkoutSection: $('checkout-section'), form: $('checkout-form'), name: $('customer-name'),
  formError: $('form-error'), placeOrder: $('place-order'),
  confirmation: $('confirmation'), confirmHeading: $('confirmation-heading'), confirmName: $('confirm-name'),
  orderReference: $('order-reference'), confirmTotal: $('confirm-total'), newOrder: $('new-order'),
};
const rows = new Map(); // productId -> { tr, input, dec, inc, lineTotal, error, remove }

const product = (id) => state.products.find((p) => p.id === id);
const locked = () => state.status !== 'idle';
const orderTotal = () => {
  let sum = 0;
  for (const [id, line] of state.cart) sum += product(id).pricePaise * line.qty;
  return sum;
};
const hasInvalidQty = () => [...state.cart.values()].some((l) => l.invalid);

function parseQty(text) {
  const t = String(text).trim();
  if (!/^\d+$/.test(t)) return null; // rejects '', '2.5', '-1', '1e1'
  const n = Number(t);
  return n >= MIN_QTY && n <= MAX_QTY ? n : null;
}

// Any change to the cart means a new checkout attempt (new idempotency key).
function cartChanged() {
  state.attemptKey = null;
  state.serverMessage = '';
}

// ---------- cart actions ----------
function addToCart(id) {
  if (locked()) return;
  const line = state.cart.get(id);
  if (!line) {
    state.cart.set(id, { qty: 1, invalid: false });
  } else if (!line.invalid && line.qty < MAX_QTY) {
    line.qty += 1;
  }
  cartChanged();
  render();
}

function setQty(id, qty) {
  const line = state.cart.get(id);
  if (!line || locked()) return;
  line.qty = qty;
  line.invalid = false;
  cartChanged();
  render();
}

// Called on every keystroke: totals update live, without re-creating the input.
function onQtyTyped(id, text) {
  const line = state.cart.get(id);
  if (!line || locked()) return;
  const n = parseQty(text);
  if (n === null) {
    line.invalid = true; // totals show '—' and checkout is blocked until fixed
  } else {
    line.qty = n;
    line.invalid = false;
  }
  cartChanged();
  render({ keepInputValue: id });
}

function removeFromCart(id) {
  if (locked()) return;
  state.cart.delete(id);
  cartChanged();
  render();
}

// ---------- checkout ----------
function clientErrors() {
  const errors = [];
  if (state.cart.size === 0) errors.push(MSG.emptyCart);
  if (els.name.value.trim() === '') errors.push(MSG.nameMissing);
  if (hasInvalidQty()) errors.push(MSG.fixQty);
  return errors;
}

function newKey() {
  if (crypto.randomUUID) return crypto.randomUUID();
  const b = crypto.getRandomValues(new Uint8Array(16));
  return Array.from(b, (x) => x.toString(16).padStart(2, '0')).join('');
}

async function handleSubmit(event) {
  event.preventDefault();
  if (state.status !== 'idle') return; // guard 1: ignore submits while one is in flight

  const errors = clientErrors();
  state.showErrors = errors.length > 0;
  state.serverMessage = '';
  if (errors.length) {
    render();
    if (els.name.value.trim() === '') els.name.focus();
    return;
  }

  state.status = 'submitting'; // set synchronously, before any await
  state.attemptKey ??= newKey(); // guard 2: same key on retry -> server returns the same order
  render();

  const body = {
    idempotencyKey: state.attemptKey,
    customerName: els.name.value.trim(),
    items: [...state.cart].map(([productId, l]) => ({ productId, qty: l.qty })),
    expectedTotalPaise: orderTotal(),
  };

  try {
    const res = await fetch('/api/orders', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    const data = await res.json().catch(() => ({}));
    if (res.ok) {
      showConfirmation(data.order);
      return;
    }
    state.status = 'idle';
    if (res.status === 422 && Array.isArray(data.errors)) {
      state.serverMessage = data.errors.map((e) => e.message).join(' ');
    } else {
      state.serverMessage = data.message || MSG.network;
    }
    if (data.error === 'price_changed') await loadProducts();
  } catch {
    state.status = 'idle'; // keep attemptKey so a retry cannot create a second order
    state.serverMessage = MSG.network;
  }
  render();
}

function showConfirmation(order) {
  state.status = 'placed';
  els.confirmName.textContent = order.customerName;
  els.orderReference.textContent = order.reference;
  els.confirmTotal.textContent = formatMoney(order.totalPaise);
  render();
  els.confirmHeading.focus();
}

function startNewOrder() {
  state.cart.clear();
  state.status = 'idle';
  state.showErrors = false;
  cartChanged();
  els.name.value = '';
  render();
  els.name.focus();
}

// ---------- rendering ----------
function renderProducts() {
  if (els.productList.childElementCount === 0) {
    for (const p of state.products) {
      const li = document.createElement('li');
      const label = document.createElement('span');
      label.textContent = `${p.name} — ${formatMoney(p.pricePaise)}`;
      const btn = document.createElement('button');
      btn.type = 'button';
      btn.textContent = 'Add to cart';
      btn.setAttribute('aria-label', `Add ${p.name} to cart`);
      btn.addEventListener('click', () => addToCart(p.id));
      li.append(label, btn);
      els.productList.append(li);
    }
  }
  for (const btn of els.productList.querySelectorAll('button')) btn.disabled = locked();
}

function createRow(id) {
  const p = product(id);
  const tr = document.createElement('tr');
  tr.dataset.testid = `cart-row-${id}`;
  const cell = (label) => { const td = document.createElement('td'); td.dataset.label = label; return td; };

  const nameTd = cell('Item'); nameTd.textContent = p.name;
  const priceTd = cell('Unit price'); priceTd.textContent = formatMoney(p.pricePaise);

  const qtyTd = cell('Quantity');
  const wrap = document.createElement('div'); wrap.className = 'qty';
  const dec = document.createElement('button');
  dec.type = 'button'; dec.textContent = '−'; dec.setAttribute('aria-label', `Decrease quantity of ${p.name}`);
  dec.addEventListener('click', () => setQty(id, state.cart.get(id).qty - 1));
  const input = document.createElement('input');
  Object.assign(input, { type: 'text', inputMode: 'numeric', autocomplete: 'off' });
  input.setAttribute('aria-label', `Quantity of ${p.name}`);
  input.addEventListener('input', () => onQtyTyped(id, input.value));
  const inc = document.createElement('button');
  inc.type = 'button'; inc.textContent = '+'; inc.setAttribute('aria-label', `Increase quantity of ${p.name}`);
  inc.addEventListener('click', () => setQty(id, state.cart.get(id).qty + 1));
  wrap.append(dec, input, inc);
  const error = document.createElement('span');
  error.className = 'qty-error';
  error.id = `qty-error-${id}`;
  error.setAttribute('aria-live', 'polite');
  input.setAttribute('aria-describedby', error.id);
  qtyTd.append(wrap, error);

  const lineTotal = cell('Line total'); lineTotal.dataset.testid = `line-total-${id}`;
  const actionTd = cell('');
  const remove = document.createElement('button');
  remove.type = 'button'; remove.textContent = 'Remove'; remove.setAttribute('aria-label', `Remove ${p.name}`);
  remove.addEventListener('click', () => removeFromCart(id));
  actionTd.append(remove);

  tr.append(nameTd, priceTd, qtyTd, lineTotal, actionTd);
  els.cartBody.append(tr);
  const row = { tr, input, dec, inc, lineTotal, error, remove };
  rows.set(id, row);
  return row;
}

function renderCart({ keepInputValue } = {}) {
  for (const [id, row] of rows) {
    if (!state.cart.has(id)) { row.tr.remove(); rows.delete(id); }
  }
  for (const [id, line] of state.cart) {
    const row = rows.get(id) || createRow(id);
    if (id !== keepInputValue && !line.invalid) row.input.value = String(line.qty);
    row.input.setAttribute('aria-invalid', String(line.invalid));
    row.error.textContent = line.invalid ? MSG.badQty : '';
    // While a quantity is invalid, show no number rather than one that doesn't match the input.
    row.lineTotal.textContent = line.invalid ? '—' : formatMoney(product(id).pricePaise * line.qty);
    row.input.readOnly = locked();
    row.dec.disabled = locked() || line.invalid || line.qty <= MIN_QTY;
    row.inc.disabled = locked() || line.invalid || line.qty >= MAX_QTY;
    row.remove.disabled = locked();
  }
  const empty = state.cart.size === 0;
  els.cartEmpty.hidden = !empty;
  els.cartTable.hidden = empty;
  els.orderTotal.textContent = hasInvalidQty() ? '—' : formatMoney(orderTotal());
}

function renderCheckout() {
  let messages = state.showErrors ? clientErrors() : [];
  if (state.showErrors && messages.length === 0) state.showErrors = false;
  if (state.serverMessage) messages = [...messages, state.serverMessage];
  els.formError.textContent = messages.join(' ');
  els.name.setAttribute('aria-invalid', String(state.showErrors && els.name.value.trim() === ''));

  els.placeOrder.disabled = state.status !== 'idle';
  els.placeOrder.textContent = state.status === 'submitting' ? 'Placing order…' : 'Place order';
  els.name.readOnly = state.status !== 'idle';
  els.checkoutSection.hidden = state.status === 'placed';
  els.confirmation.hidden = state.status !== 'placed';
}

function render(opts) {
  renderProducts();
  renderCart(opts);
  renderCheckout();
}

async function loadProducts() {
  try {
    const res = await fetch('/api/products');
    if (!res.ok) throw new Error(String(res.status));
    state.products = (await res.json()).products;
    els.productList.replaceChildren();
    els.productsStatus.hidden = true;
  } catch {
    els.productsStatus.textContent = "Products couldn't be loaded. Please refresh the page.";
  }
  render();
}

els.form.addEventListener('submit', handleSubmit);
els.name.addEventListener('input', () => { if (state.showErrors) renderCheckout(); });
els.newOrder.addEventListener('click', startNewOrder);
loadProducts();
