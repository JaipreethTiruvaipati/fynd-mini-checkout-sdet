//go:build e2e

// Package e2e drives the real checkout page in real browsers (Chromium, Firefox,
// WebKit and a mobile Chromium profile) using playwright-go.
//
//	go test -tags e2e ./e2e -v
//	BROWSERS=webkit go test -tags e2e ./e2e -v      # one engine
//	HEADED=1 go test -tags e2e ./e2e -run TC_01 -v  # watch it
package e2e

import (
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mxschmitt/playwright-go"

	"github.com/JaipreethTiruvaipati/fynd-mini-checkout-sdet/internal/checkout"
	"github.com/JaipreethTiruvaipati/fynd-mini-checkout-sdet/internal/web"
)

// orderDelay makes order placement take as long as a real network + payment call,
// so double-submits genuinely race against an in-flight order.
const orderDelay = 300 * time.Millisecond

type project struct {
	name    string
	browser playwright.Browser
	device  *playwright.DeviceDescriptor // nil for desktop
}

var (
	pw       *playwright.Playwright
	projects []project
)

func TestMain(m *testing.M) {
	var err error
	pw, err = playwright.Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, "playwright:", err, "\nInstall browsers: go run github.com/mxschmitt/playwright-go/cmd/playwright@v0.6401.0 install")
		os.Exit(1)
	}
	headless := os.Getenv("HEADED") == ""
	want := os.Getenv("BROWSERS")
	if want == "" {
		want = "chromium,firefox,webkit,mobile-chrome"
	}
	for _, name := range strings.Split(want, ",") {
		bt, device := pw.Chromium, (*playwright.DeviceDescriptor)(nil)
		switch strings.TrimSpace(name) {
		case "firefox":
			bt = pw.Firefox
		case "webkit":
			bt = pw.WebKit
		case "mobile-chrome":
			device = pw.Devices["Pixel 7"]
		}
		b, err := bt.Launch(playwright.BrowserTypeLaunchOptions{Headless: playwright.Bool(headless)})
		if err != nil {
			fmt.Fprintln(os.Stderr, "launch", name, err)
			os.Exit(1)
		}
		projects = append(projects, project{name: strings.TrimSpace(name), browser: b, device: device})
	}
	code := m.Run()
	for _, p := range projects {
		_ = p.browser.Close()
	}
	_ = pw.Stop()
	os.Exit(code)
}

// h is the per-test harness: a fresh server, a fresh browser context and page.
type h struct {
	t       *testing.T
	page    playwright.Page
	svc     *checkout.Service
	expect  playwright.PlaywrightAssertions
	project string
}

// each runs fn once per browser project, every time against a brand-new server and
// browser context, so no state (orders, cookies, storage) leaks between tests.
func each(t *testing.T, fn func(x *h)) {
	for _, p := range projects {
		t.Run(p.name, func(t *testing.T) {
			svc := checkout.NewService(orderDelay)
			srv := httptest.NewServer(web.NewHandler(svc, slog.New(slog.NewTextHandler(io.Discard, nil))))
			defer srv.Close()

			opts := playwright.BrowserNewContextOptions{BaseURL: playwright.String(srv.URL)}
			if d := p.device; d != nil {
				opts.UserAgent = playwright.String(d.UserAgent)
				opts.Viewport = d.Viewport
				opts.DeviceScaleFactor = playwright.Float(d.DeviceScaleFactor)
				opts.IsMobile = playwright.Bool(d.IsMobile)
				opts.HasTouch = playwright.Bool(d.HasTouch)
			}
			ctx, err := p.browser.NewContext(opts)
			if err != nil {
				t.Fatal(err)
			}
			defer ctx.Close()
			page, err := ctx.NewPage()
			if err != nil {
				t.Fatal(err)
			}
			x := &h{t: t, page: page, svc: svc, expect: playwright.NewPlaywrightAssertions(5000), project: p.name}

			// Browser-side event log, printed only when a test fails, so a flaky
			// failure can be diagnosed from CI output alone.
			var evMu sync.Mutex
			var events []string
			started := time.Now()
			logEv := func(format string, a ...any) {
				evMu.Lock()
				defer evMu.Unlock()
				events = append(events, fmt.Sprintf("%6dms  %s", time.Since(started).Milliseconds(), fmt.Sprintf(format, a...)))
			}
			page.OnRequest(func(r playwright.Request) { logEv("→ %s %s", r.Method(), r.URL()) })
			page.OnResponse(func(r playwright.Response) { logEv("← %d %s", r.Status(), r.URL()) })
			page.OnRequestFailed(func(r playwright.Request) { logEv("✗ %s %s: %v", r.Method(), r.URL(), r.Failure()) })
			page.OnConsole(func(m playwright.ConsoleMessage) { logEv("console.%s: %s", m.Type(), m.Text()) })
			page.OnPageError(func(err error) { logEv("page error: %v", err) })

			defer func() {
				if t.Failed() {
					evMu.Lock()
					t.Logf("browser events:\n%s", strings.Join(events, "\n"))
					evMu.Unlock()
					path := fmt.Sprintf("../test-results/%s-%s.png", strings.ReplaceAll(t.Name(), "/", "-"), p.name)
					_, _ = page.Screenshot(playwright.PageScreenshotOptions{Path: playwright.String(path), FullPage: playwright.Bool(true)})
					t.Logf("failure screenshot: %s", path)
					t.Logf("server state: %d order request(s) received, %d order(s) created", svc.RequestsSeen(), len(svc.Orders()))
				}
			}()
			x.openCleanCheckout()
			fn(x)
		})
	}
}

// ok fails the test immediately with a description if err is not nil.
func (x *h) ok(err error, what string, args ...any) {
	x.t.Helper()
	if err != nil {
		x.t.Fatalf("%s: %v", fmt.Sprintf(what, args...), err)
	}
}

// openCleanCheckout loads the page and asserts the clean starting state every test relies on.
func (x *h) openCleanCheckout() {
	x.t.Helper()
	_, err := x.page.Goto("/")
	x.ok(err, "open checkout")
	x.ok(x.expect.Locator(x.addButton("A5 Dotted Notebook")).ToBeVisible(), "products loaded")
	x.ok(x.expect.Locator(x.page.GetByText("Your cart is empty.")).ToBeVisible(), "cart starts empty")
	x.ok(x.expect.Locator(x.orderTotal()).ToHaveText("₹0.00"), "total starts at ₹0.00")
	x.ok(x.expect.Locator(x.name()).ToHaveValue(""), "name starts empty")
	x.ok(x.expect.Locator(x.confirmationHeading()).ToBeHidden(), "no confirmation at start")
}

// ---------- locators: by role and label, the way a customer finds things ----------

func (x *h) button(name string) playwright.Locator {
	return x.page.GetByRole(*playwright.AriaRoleButton, playwright.PageGetByRoleOptions{Name: name, Exact: playwright.Bool(true)})
}
func (x *h) addButton(product string) playwright.Locator { return x.button("Add " + product + " to cart") }
func (x *h) placeOrder() playwright.Locator {
	return x.page.Locator("#place-order") // its label changes to "Placing order…" while submitting
}
func (x *h) name() playwright.Locator { return x.page.GetByLabel("Full name") }
func (x *h) qty(product string) playwright.Locator {
	return x.page.GetByRole(*playwright.AriaRoleTextbox, playwright.PageGetByRoleOptions{Name: "Quantity of " + product, Exact: playwright.Bool(true)})
}
func (x *h) formError() playwright.Locator          { return x.page.Locator("#form-error") }
func (x *h) orderTotal() playwright.Locator         { return x.page.GetByTestId("order-total") }
func (x *h) lineTotal(id string) playwright.Locator { return x.page.GetByTestId("line-total-" + id) }
func (x *h) confirmationHeading() playwright.Locator {
	return x.page.GetByRole(*playwright.AriaRoleHeading, playwright.PageGetByRoleOptions{Name: "Order confirmed"})
}
func (x *h) orderReference() playwright.Locator { return x.page.GetByTestId("order-reference") }
func (x *h) confirmTotal() playwright.Locator   { return x.page.GetByTestId("confirm-total") }

// ---------- actions ----------

func (x *h) click(l playwright.Locator, what string) {
	x.t.Helper()
	x.ok(l.Click(), "click %s", what)
}
func (x *h) fill(l playwright.Locator, v, what string) {
	x.t.Helper()
	x.ok(l.Fill(v), "fill %s with %q", what, v)
}

// ---------- server-side assertions (the source of truth for "how many orders") ----------

func (x *h) wantOrders(n int) {
	x.t.Helper()
	if got := len(x.svc.Orders()); got != n {
		x.t.Fatalf("server has %d order(s), want %d", got, n)
	}
}
func (x *h) wantRequests(n int) {
	x.t.Helper()
	if got := x.svc.RequestsSeen(); got != n {
		x.t.Fatalf("server received %d order request(s), want %d", got, n)
	}
}
