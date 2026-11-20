'use strict';

/*
 * Mini Checkout — single page, no backend.
 * Money is stored as integer paise to avoid floating-point rounding errors
 * (e.g. 3 x 149.75 must be exactly 449.25).
 */

const PRODUCTS = [
  { id: 'notebook', name: 'A5 Dotted Notebook', pricePaise: 14975 }, // ₹149.75
  { id: 'lamp', name: 'LED Desk Lamp', pricePaise: 129900 },          // ₹1,299.00
];
const MIN_QTY = 1;
const MAX_QTY = 10;
const PROCESSING_DELAY_MS = 600; // simulates a network round-trip so double-submit is a real risk

const inr = new Intl.NumberFormat('en-IN', { style: 'currency', currency: 'INR', minimumFractionDigits: 2 });
const formatMoney = (paise) => inr.format(paise / 100);

const state = {
  cart: [],          // [{ id, qty }] in the order items were added
  status: 'idle',    // 'idle' | 'submitting' | 'placed'
  order: null,
  ordersPlaced: 0,   // instrumentation: how many orders this page has created
  qtyErrors: {},     // { [productId]: message }
  showFormErrors: false,
};

const $ = (id) => document.getElementById(id);
const els = {
  productList: $('product-list'),
  cartEmpty: $('cart-empty'),
  cartTable: $('cart-table'),
  cartBody: $('cart-body'),
  orderTotal: $('order-total'),
  checkoutSection: $('checkout-section'),
  form: $('checkout-form'),
  name: $('customer-name'),
  formError: $('form-error'),
  placeOrder: $('place-order'),
  confirmation: $('confirmation'),
  confirmHeading: $('confirmation-heading'),
  confirmName: $('confirm-name'),
  orderReference: $('order-reference'),
  confirmTotal: $('confirm-total'),
  newOrder: $('new-order'),
};

const productById = (id) => PRODUCTS.find((p) => p.id === id);
const cartLine = (id) => state.cart.find((l) => l.id === id);
const lineTotal = (line) => productById(line.id).pricePaise * line.qty;
const orderTotal = () => state.cart.reduce((sum, l) => sum + lineTotal(l), 0);
const locked = () => state.status !== 'idle';

// ---------- cart actions ----------
function addToCart(id) {
  if (locked()) return;
  const line = cartLine(id);
  if (line) {
    if (line.qty >= MAX_QTY) {
      state.qtyErrors[id] = `Maximum ${MAX_QTY} per item.`;
    } else {
      line.qty += 1;
      delete state.qtyErrors[id];
    }
  } else {
    state.cart.push({ id, qty: 1 });
  }
  render();
}

function setQty(id, rawValue) {
  if (locked()) return;
  const line = cartLine(id);
  if (!line) return;
  const text = String(rawValue).trim();
  const n = Number(text);
  if (text === '' || !Number.isInteger(n) || n < MIN_QTY || n > MAX_QTY) {
    // Reject invalid input and keep the last valid quantity.
    state.qtyErrors[id] = `Enter a whole number from ${MIN_QTY} to ${MAX_QTY}.`;
  } else {
    line.qty = n;
    delete state.qtyErrors[id];
  }
  render();
}

function removeFromCart(id) {
  if (locked()) return;
  state.cart = state.cart.filter((l) => l.id !== id);
  delete state.qtyErrors[id];
  render();
}

// ---------- validation & checkout ----------
function validate() {
  const errors = [];
  if (state.cart.length === 0) {
    errors.push('Your cart is empty. Add at least one item before placing your order.');
  }
  if (els.name.value.trim() === '') {
    errors.push('Please enter your full name.');
  }
  // DEF-01: a rejected quantity edit must block checkout, otherwise the order is
  // placed with a quantity the customer did not confirm while an error is on screen.
  if (Object.keys(state.qtyErrors).length > 0) {
    errors.push('Fix the highlighted quantity before placing your order.');
  }
  return errors;
}

function generateReference() {
  const bytes = new Uint8Array(6);
  crypto.getRandomValues(bytes);
  const chars = 'ABCDEFGHJKLMNPQRSTUVWXYZ23456789'; // no 0/O/1/I to avoid misreading
  return 'ORD-' + Array.from(bytes, (b) => chars[b % chars.length]).join('');
}

function handleSubmit(event) {
  event.preventDefault();
  // Idempotency guard: once a submission is in flight or done, ignore further submits.
  if (state.status !== 'idle') return;

  const errors = validate();
  state.showFormErrors = errors.length > 0;
  if (errors.length > 0) {
    render();
    if (els.name.value.trim() === '') els.name.focus();
    return; // the name the customer typed and the cart are left untouched
  }

  state.status = 'submitting'; // set synchronously, before any async work
  render();

  setTimeout(() => {
    state.ordersPlaced += 1;
    state.order = {
      reference: generateReference(),
      name: els.name.value.trim(),
      totalPaise: orderTotal(),
      items: state.cart.map((l) => ({ ...l })),
    };
    state.status = 'placed';
    render();
    els.confirmHeading.focus();
  }, PROCESSING_DELAY_MS);
}

function startNewOrder() {
  state.cart = [];
  state.status = 'idle';
  state.order = null;
  state.qtyErrors = {};
  state.showFormErrors = false;
  els.name.value = '';
  render();
  els.name.focus();
}

// ---------- rendering ----------
function renderProducts() {
  els.productList.innerHTML = '';
  for (const p of PRODUCTS) {
    const li = document.createElement('li');
    li.dataset.testid = `product-${p.id}`;
    const label = document.createElement('span');
    label.textContent = `${p.name} — ${formatMoney(p.pricePaise)}`;
    const btn = document.createElement('button');
    btn.type = 'button';
    btn.textContent = 'Add to cart';
    btn.setAttribute('aria-label', `Add ${p.name} to cart`);
    btn.disabled = locked();
    btn.addEventListener('click', () => addToCart(p.id));
    li.append(label, btn);
    els.productList.append(li);
  }
}

function renderCart() {
  const empty = state.cart.length === 0;
  els.cartEmpty.hidden = !empty;
  els.cartTable.hidden = empty;
  els.cartBody.innerHTML = '';

  for (const line of state.cart) {
    const p = productById(line.id);
    const tr = document.createElement('tr');
    tr.dataset.testid = `cart-row-${p.id}`;

    const nameTd = document.createElement('td');
    nameTd.textContent = p.name;

    const priceTd = document.createElement('td');
    priceTd.textContent = formatMoney(p.pricePaise);

    const qtyTd = document.createElement('td');
    const qtyWrap = document.createElement('div');
    qtyWrap.className = 'qty';
    const dec = document.createElement('button');
    dec.type = 'button';
    dec.textContent = '−';
    dec.setAttribute('aria-label', `Decrease quantity of ${p.name}`);
    dec.disabled = locked() || line.qty <= MIN_QTY;
    dec.addEventListener('click', () => setQty(p.id, line.qty - 1));
    const input = document.createElement('input');
    input.type = 'number';
    input.min = String(MIN_QTY);
    input.max = String(MAX_QTY);
    input.step = '1';
    input.value = String(line.qty);
    input.setAttribute('aria-label', `Quantity of ${p.name}`);
    input.disabled = locked();
    input.addEventListener('change', () => setQty(p.id, input.value));
    const inc = document.createElement('button');
    inc.type = 'button';
    inc.textContent = '+';
    inc.setAttribute('aria-label', `Increase quantity of ${p.name}`);
    inc.disabled = locked() || line.qty >= MAX_QTY;
    inc.addEventListener('click', () => setQty(p.id, line.qty + 1));
    qtyWrap.append(dec, input, inc);
    qtyTd.append(qtyWrap);
    if (state.qtyErrors[p.id]) {
      const err = document.createElement('span');
      err.className = 'qty-error';
      err.setAttribute('role', 'alert');
      err.textContent = state.qtyErrors[p.id];
      qtyTd.append(err);
      input.setAttribute('aria-invalid', 'true');
    }

    const totalTd = document.createElement('td');
    totalTd.dataset.testid = `line-total-${p.id}`;
    totalTd.textContent = formatMoney(lineTotal(line));

    const actionTd = document.createElement('td');
    const rm = document.createElement('button');
    rm.type = 'button';
    rm.textContent = 'Remove';
    rm.setAttribute('aria-label', `Remove ${p.name}`);
    rm.disabled = locked();
    rm.addEventListener('click', () => removeFromCart(p.id));
    actionTd.append(rm);

    tr.append(nameTd, priceTd, qtyTd, totalTd, actionTd);
    els.cartBody.append(tr);
  }

  els.orderTotal.textContent = formatMoney(orderTotal());
}

function renderCheckout() {
  const errors = state.showFormErrors ? validate() : [];
  if (state.showFormErrors && errors.length === 0) state.showFormErrors = false;
  els.formError.textContent = errors.join(' ');
  els.name.setAttribute('aria-invalid', String(state.showFormErrors && els.name.value.trim() === ''));

  els.placeOrder.disabled = state.status !== 'idle';
  els.placeOrder.textContent = state.status === 'submitting' ? 'Placing order…' : 'Place order';
  els.name.readOnly = state.status !== 'idle';
  els.checkoutSection.hidden = state.status === 'placed';

  const placed = state.status === 'placed';
  els.confirmation.hidden = !placed;
  if (placed) {
    els.confirmName.textContent = state.order.name;
    els.orderReference.textContent = state.order.reference;
    els.confirmTotal.textContent = formatMoney(state.order.totalPaise);
  }
  document.body.dataset.ordersPlaced = String(state.ordersPlaced);
}

function render() {
  renderProducts();
  renderCart();
  renderCheckout();
}

els.form.addEventListener('submit', handleSubmit);
els.name.addEventListener('input', () => { if (state.showFormErrors) renderCheckout(); });
els.newOrder.addEventListener('click', startNewOrder);
render();
