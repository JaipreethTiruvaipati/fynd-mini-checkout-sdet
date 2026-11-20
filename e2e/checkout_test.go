//go:build e2e

package e2e

import (
	"regexp"
	"testing"

	"github.com/mxschmitt/playwright-go"

	"github.com/JaipreethTiruvaipati/fynd-mini-checkout-sdet/internal/checkout"
)

var refPattern = regexp.MustCompile(`^ORD-[A-HJ-NP-Z2-9]{6}$`)

// ===================== The three required checks =====================

// TC-01: a successful checkout produces exactly one confirmation with a reference,
// and the server recorded exactly that order for exactly the amount shown.
func TestTC_01_SuccessfulCheckoutShowsOneConfirmation(t *testing.T) {
	each(t, func(x *h) {
		x.click(x.addButton("A5 Dotted Notebook"), "add notebook")
		x.click(x.addButton("LED Desk Lamp"), "add lamp")
		x.ok(x.expect.Locator(x.orderTotal()).ToHaveText("₹1,448.75"), "cart total")

		x.fill(x.name(), "Asha Verma", "name")
		x.click(x.placeOrder(), "Place order")

		x.ok(x.expect.Locator(x.confirmationHeading()).ToBeVisible(), "confirmation shown")
		x.ok(x.expect.Locator(x.confirmationHeading()).ToHaveCount(1), "exactly one confirmation")
		x.ok(x.expect.Locator(x.page.GetByText("Thank you, Asha Verma.")).ToBeVisible(), "customer name on confirmation")
		x.ok(x.expect.Locator(x.orderReference()).ToHaveText(refPattern), "reference format")
		x.ok(x.expect.Locator(x.confirmTotal()).ToHaveText("₹1,448.75"), "confirmed amount")
		x.ok(x.expect.Locator(x.placeOrder()).ToBeHidden(), "cannot submit again")

		x.wantOrders(1)
		o := x.svc.Orders()[0]
		shown, _ := x.orderReference().TextContent()
		if o.Reference != shown || o.Total != 144875 || o.CustomerName != "Asha Verma" {
			t.Fatalf("server order %+v does not match what the customer saw (ref %s)", o, shown)
		}
	})
}

// TC-02: a missing required field blocks checkout with a clear message and keeps the cart.
func TestTC_02_MissingNameBlocksCheckout(t *testing.T) {
	for _, tc := range []struct{ label, value string }{{"blank", ""}, {"whitespace", "   "}} {
		t.Run(tc.label, func(t *testing.T) {
			each(t, func(x *h) {
				x.click(x.addButton("A5 Dotted Notebook"), "add notebook")
				x.fill(x.name(), tc.value, "name")
				x.click(x.placeOrder(), "Place order")

				x.ok(x.expect.Locator(x.formError()).ToHaveText(checkout.MsgNameMissing), "message")
				x.ok(x.expect.Locator(x.name()).ToHaveAttribute("aria-invalid", "true"), "field marked invalid")
				x.ok(x.expect.Locator(x.name()).ToBeFocused(), "focus moved to the field")
				x.ok(x.expect.Locator(x.confirmationHeading()).ToBeHidden(), "no confirmation")
				x.ok(x.expect.Locator(x.page.GetByTestId("cart-row-notebook")).ToBeVisible(), "cart kept")
				x.ok(x.expect.Locator(x.orderTotal()).ToHaveText("₹149.75"), "total kept")
				x.wantRequests(0) // blocked before anything was sent

				// Fixing the field clears the message and checkout goes through.
				x.fill(x.name(), "Asha Verma", "name")
				x.ok(x.expect.Locator(x.formError()).ToHaveText(""), "message cleared")
				x.click(x.placeOrder(), "Place order")
				x.ok(x.expect.Locator(x.confirmationHeading()).ToBeVisible(), "confirmation after fix")
				x.wantOrders(1)
			})
		})
	}
}

// TC-03: a quantity change updates the line total and the exact order total —
// and the server charges exactly that total.
func TestTC_03_QuantityChangeUpdatesExactTotal(t *testing.T) {
	each(t, func(x *h) {
		x.click(x.addButton("A5 Dotted Notebook"), "add notebook")
		x.ok(x.expect.Locator(x.lineTotal("notebook")).ToHaveText("₹149.75"), "line total qty 1")

		// 3 x ₹149.75 = ₹449.25 exactly (a float implementation shows 449.24999...).
		x.fill(x.qty("A5 Dotted Notebook"), "3", "notebook qty")
		x.ok(x.expect.Locator(x.lineTotal("notebook")).ToHaveText("₹449.25"), "line total qty 3")
		x.ok(x.expect.Locator(x.orderTotal()).ToHaveText("₹449.25"), "order total qty 3")

		x.click(x.addButton("LED Desk Lamp"), "add lamp")
		x.ok(x.expect.Locator(x.lineTotal("lamp")).ToHaveText("₹1,299.00"), "lamp line total")
		x.ok(x.expect.Locator(x.orderTotal()).ToHaveText("₹1,748.25"), "total with lamp")

		x.click(x.button("Increase quantity of A5 Dotted Notebook"), "+ notebook")
		x.ok(x.expect.Locator(x.qty("A5 Dotted Notebook")).ToHaveValue("4"), "qty 4")
		x.ok(x.expect.Locator(x.lineTotal("notebook")).ToHaveText("₹599.00"), "line total qty 4")
		x.ok(x.expect.Locator(x.orderTotal()).ToHaveText("₹1,898.00"), "total qty 4")

		x.click(x.button("Decrease quantity of A5 Dotted Notebook"), "- notebook")
		x.ok(x.expect.Locator(x.orderTotal()).ToHaveText("₹1,748.25"), "total back to qty 3")

		x.click(x.button("Remove LED Desk Lamp"), "remove lamp")
		x.ok(x.expect.Locator(x.lineTotal("lamp")).ToHaveCount(0), "lamp row removed")
		x.ok(x.expect.Locator(x.orderTotal()).ToHaveText("₹449.25"), "total after remove")

		x.fill(x.name(), "Asha Verma", "name")
		x.click(x.placeOrder(), "Place order")
		x.ok(x.expect.Locator(x.confirmTotal()).ToHaveText("₹449.25"), "confirmed amount")
		x.wantOrders(1)
		if got := x.svc.Orders()[0]; got.Total != 44925 || got.Lines[0].Qty != 3 {
			t.Fatalf("server charged %d paise for qty %d, want 44925 for qty 3", got.Total, got.Lines[0].Qty)
		}
	})
}

// ===================== Edge cases and regressions =====================

func TestEC_01_EmptyCartBlocksCheckoutAndKeepsName(t *testing.T) {
	each(t, func(x *h) {
		x.fill(x.name(), "Asha Verma", "name")
		x.click(x.placeOrder(), "Place order")
		x.ok(x.expect.Locator(x.formError()).ToHaveText(checkout.MsgEmptyCart), "message")
		x.ok(x.expect.Locator(x.name()).ToHaveValue("Asha Verma"), "name kept")
		x.ok(x.expect.Locator(x.confirmationHeading()).ToBeHidden(), "no confirmation")
		x.wantRequests(0)

		// Adding an item clears the message.
		x.click(x.addButton("LED Desk Lamp"), "add lamp")
		x.ok(x.expect.Locator(x.formError()).ToHaveText(""), "message cleared")
	})
}

func TestEC_02_EmptyCartAndBlankNameShowBothMessages(t *testing.T) {
	each(t, func(x *h) {
		x.click(x.placeOrder(), "Place order")
		x.ok(x.expect.Locator(x.formError()).ToContainText(checkout.MsgEmptyCart), "cart message")
		x.ok(x.expect.Locator(x.formError()).ToContainText(checkout.MsgNameMissing), "name message")
	})
}

func ready(x *h) {
	x.t.Helper()
	x.click(x.addButton("A5 Dotted Notebook"), "add notebook")
	x.fill(x.name(), "Asha Verma", "name")
}

// EC-03: a rapid double-click produces one request and one order.
func TestEC_03_DoubleClickPlacesOneOrder(t *testing.T) {
	each(t, func(x *h) {
		ready(x)
		x.ok(x.placeOrder().Dblclick(), "double-click Place order")
		x.ok(x.expect.Locator(x.confirmationHeading()).ToBeVisible(), "confirmation")
		x.ok(x.expect.Locator(x.confirmationHeading()).ToHaveCount(1), "one confirmation")
		// Both clicks happened before the first response arrived, so a second
		// request would already have reached the server by now.
		x.wantRequests(1)
		x.wantOrders(1)
	})
}

// EC-04: three submits in the same JavaScript tick (bypassing the disabled button).
func TestEC_04_SameTickSubmitsPlaceOneOrder(t *testing.T) {
	each(t, func(x *h) {
		ready(x)
		_, err := x.page.Evaluate(`() => { const f = document.getElementById('checkout-form'); f.requestSubmit(); f.requestSubmit(); f.requestSubmit(); }`)
		x.ok(err, "submit 3x")
		x.ok(x.expect.Locator(x.confirmationHeading()).ToBeVisible(), "confirmation")
		x.wantRequests(1)
		x.wantOrders(1)
	})
}

// EC-05: Enter pressed repeatedly in the name field.
func TestEC_05_RepeatedEnterPlacesOneOrder(t *testing.T) {
	each(t, func(x *h) {
		ready(x)
		for range 3 {
			x.ok(x.name().Press("Enter"), "press Enter")
		}
		x.ok(x.expect.Locator(x.confirmationHeading()).ToBeVisible(), "confirmation")
		x.wantOrders(1)
	})
}

// EC-06: the order is created but the response never reaches the browser (timeout,
// dropped connection). The customer clicks Place order again. The retry reuses the
// idempotency key, so the server returns the original order instead of charging twice.
func TestEC_06_LostResponseRetryDoesNotCreateSecondOrder(t *testing.T) {
	each(t, func(x *h) {
		ready(x)
		calls := 0
		x.ok(x.page.Route("**/api/orders", func(r playwright.Route) {
			calls++
			if calls == 1 {
				if _, err := r.Fetch(); err != nil { // reaches the server: order IS created
					t.Errorf("forward first request: %v", err)
				}
				_ = r.Abort("failed") // ...but the browser never sees the response
				return
			}
			_ = r.Continue()
		}), "intercept orders")

		x.click(x.placeOrder(), "Place order (response will be lost)")
		x.ok(x.expect.Locator(x.formError()).ToContainText("couldn't reach the store"), "network error shown")
		x.ok(x.expect.Locator(x.confirmationHeading()).ToBeHidden(), "no confirmation yet")
		x.wantOrders(1) // the server did create it

		x.click(x.placeOrder(), "Place order again")
		x.ok(x.expect.Locator(x.confirmationHeading()).ToBeVisible(), "confirmation after retry")
		x.wantRequests(2)
		x.wantOrders(1)
		shown, _ := x.orderReference().TextContent()
		if shown != x.svc.Orders()[0].Reference {
			t.Fatalf("customer sees %s but the order is %s", shown, x.svc.Orders()[0].Reference)
		}
	})
}

// EC-07: invalid quantities show a message, keep what was typed, hide the totals and block checkout.
func TestEC_07_InvalidQuantityIsRejected(t *testing.T) {
	each(t, func(x *h) {
		ready(x)
		qty := x.qty("A5 Dotted Notebook")
		for _, bad := range []string{"0", "11", "2.5", "-1", "", "abc", "1e1"} {
			x.fill(qty, bad, "qty")
			x.ok(x.expect.Locator(x.page.Locator("#qty-error-notebook")).ToHaveText(checkout.MsgBadQty), "qty error for %q", bad)
			x.ok(x.expect.Locator(qty).ToHaveAttribute("aria-invalid", "true"), "qty invalid for %q", bad)
			x.ok(x.expect.Locator(qty).ToHaveValue(bad), "customer's input kept for %q", bad)
			x.ok(x.expect.Locator(x.lineTotal("notebook")).ToHaveText("—"), "no line total for %q", bad)
			x.ok(x.expect.Locator(x.orderTotal()).ToHaveText("—"), "no order total for %q", bad)
		}
		x.click(x.placeOrder(), "Place order")
		x.ok(x.expect.Locator(x.formError()).ToHaveText("Fix the highlighted quantity before placing your order."), "blocked")
		x.wantRequests(0)

		x.fill(qty, "2", "qty")
		x.ok(x.expect.Locator(x.page.Locator("#qty-error-notebook")).ToHaveText(""), "error cleared")
		x.ok(x.expect.Locator(x.orderTotal()).ToHaveText("₹299.50"), "total for qty 2")
		x.ok(x.expect.Locator(x.formError()).ToHaveText(""), "form message cleared")
	})
}

// EC-08 (DEF-01 + DEF-02 regression): type a quantity and click Place order straight
// away. Before the fix, Chromium placed an order despite an error (DEF-01), and
// Firefox/WebKit silently ignored the click (DEF-02).
func TestEC_08_TypeQuantityThenClickPlaceOrderImmediately(t *testing.T) {
	t.Run("valid quantity is used", func(t *testing.T) {
		each(t, func(x *h) {
			ready(x)
			x.fill(x.qty("A5 Dotted Notebook"), "3", "qty")
			x.click(x.placeOrder(), "Place order") // single click, no Tab/blur first
			x.ok(x.expect.Locator(x.confirmTotal()).ToHaveText("₹449.25"), "charged for qty 3")
			x.wantOrders(1)
		})
	})
	t.Run("invalid quantity blocks", func(t *testing.T) {
		each(t, func(x *h) {
			ready(x)
			x.fill(x.qty("A5 Dotted Notebook"), "0", "qty")
			x.click(x.placeOrder(), "Place order")
			x.ok(x.expect.Locator(x.formError()).ToHaveText("Fix the highlighted quantity before placing your order."), "blocked with message")
			x.ok(x.expect.Locator(x.confirmationHeading()).ToBeHidden(), "no confirmation")
			x.wantRequests(0)
		})
	})
}

// EC-09: "Start a new order" returns to a clean state and the next order is separate.
func TestEC_09_StartNewOrderResets(t *testing.T) {
	each(t, func(x *h) {
		ready(x)
		x.click(x.placeOrder(), "Place order")
		x.ok(x.expect.Locator(x.confirmationHeading()).ToBeVisible(), "first confirmation")
		first, _ := x.orderReference().TextContent()

		x.click(x.button("Start a new order"), "Start a new order")
		x.ok(x.expect.Locator(x.page.GetByText("Your cart is empty.")).ToBeVisible(), "cart empty")
		x.ok(x.expect.Locator(x.orderTotal()).ToHaveText("₹0.00"), "total reset")
		x.ok(x.expect.Locator(x.name()).ToHaveValue(""), "name reset")

		ready(x)
		x.click(x.placeOrder(), "Place second order")
		x.ok(x.expect.Locator(x.orderReference()).Not().ToHaveText(first), "new reference")
		x.wantOrders(2)
	})
}

// EC-10 (DEF-03 regression): nothing in the cart is off-screen; Remove is reachable.
func TestEC_10_CartFitsTheScreen(t *testing.T) {
	each(t, func(x *h) {
		x.click(x.addButton("A5 Dotted Notebook"), "add notebook")
		x.click(x.addButton("LED Desk Lamp"), "add lamp")
		overflow, err := x.page.Evaluate(`() => document.documentElement.scrollWidth - document.documentElement.clientWidth`)
		x.ok(err, "measure overflow")
		if n, _ := overflow.(int); n > 0 {
			t.Fatalf("page scrolls sideways by %dpx on %s", n, x.project)
		}
		x.ok(x.expect.Locator(x.button("Remove LED Desk Lamp")).ToBeInViewport(), "Remove is on screen")
		x.click(x.button("Remove LED Desk Lamp"), "remove lamp")
		x.ok(x.expect.Locator(x.orderTotal()).ToHaveText("₹149.75"), "total after remove")
	})
}
