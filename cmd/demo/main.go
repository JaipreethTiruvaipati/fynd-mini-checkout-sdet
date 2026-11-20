// Command demo records the product demo video and README screenshots by driving
// the real app (real Go server, real browser). Nothing in the video is mocked.
//
//	go run ./cmd/demo     # writes frames to docs/demo/raw/ and docs/screenshots/*.png
//	./scripts/make-demo-video.sh   # stitches the clips into docs/demo/mini-checkout-demo.mp4
package main

import (
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"

	"github.com/mxschmitt/playwright-go"

	"github.com/JaipreethTiruvaipati/fynd-mini-checkout-sdet/internal/checkout"
	"github.com/JaipreethTiruvaipati/fynd-mini-checkout-sdet/internal/web"
)

const (
	shotsDir = "docs/screenshots"
	rawDir   = "docs/demo/raw"
)

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

type clip struct {
	name string
	ctx  playwright.BrowserContext
	page playwright.Page
	stop chan struct{}
	done chan struct{}
}

// record grabs a frame roughly every 100ms with its real timestamp, so the final video
// plays at true speed (Playwright's own recorder drops frames when the page is still).
// Frames go to docs/demo/raw/<clip>/NNNNN.png plus an ffmpeg concat list with durations.
func (c *clip) record() {
	dir := filepath.Join(rawDir, c.name)
	must(os.MkdirAll(dir, 0o755))
	c.stop, c.done = make(chan struct{}), make(chan struct{})
	go func() {
		defer close(c.done)
		type frame struct {
			file string
			at   time.Time
		}
		var frames []frame
		for i := 0; ; i++ {
			select {
			case <-c.stop:
				f, err := os.Create(filepath.Join(dir, "frames.txt"))
				must(err)
				defer f.Close()
				for j, fr := range frames {
					next := time.Now()
					if j+1 < len(frames) {
						next = frames[j+1].at
					}
					fmt.Fprintf(f, "file '%s'\nduration %.3f\n", fr.file, next.Sub(fr.at).Seconds())
				}
				if len(frames) > 0 { // concat demuxer needs the last file repeated
					fmt.Fprintf(f, "file '%s'\n", frames[len(frames)-1].file)
				}
				return
			default:
			}
			at := time.Now()
			name := fmt.Sprintf("%05d.png", i)
			if _, err := c.page.Screenshot(playwright.PageScreenshotOptions{Path: playwright.String(filepath.Join(dir, name))}); err == nil {
				frames = append(frames, frame{name, at})
			}
			if d := 100*time.Millisecond - time.Since(at); d > 0 {
				time.Sleep(d)
			}
		}
	}()
}

// newClip opens a page whose video is saved as docs/demo/raw/<name>.webm.
func newClip(b playwright.Browser, baseURL, name string, w, h int, mobile bool) *clip {
	size := &playwright.Size{Width: w, Height: h}
	opts := playwright.BrowserNewContextOptions{Viewport: size, DeviceScaleFactor: playwright.Float(1)}
	if baseURL != "" {
		opts.BaseURL = playwright.String(baseURL)
	}
	if mobile {
		opts.IsMobile = playwright.Bool(true)
		opts.HasTouch = playwright.Bool(true)
	}
	ctx, err := b.NewContext(opts)
	must(err)
	page, err := ctx.NewPage()
	must(err)
	return &clip{name: name, ctx: ctx, page: page}
}

func (c *clip) goTo(url string) {
	_, err := c.page.Goto(url)
	must(err)
	_, err = c.page.Evaluate(`() => {
		const s = document.createElement('style');
		s.textContent = '@keyframes __bar{from{width:0}to{width:100%}}';
		document.head.appendChild(s);
	}`)
	must(err)
	if c.stop == nil {
		c.record()
	}
}

// caption shows a subtitle (demo overlay only, not part of the app) for hold, with a
// progress bar so viewers know how long it stays up. holdMs 0 leaves it on screen.
func (c *clip) caption(text string, holdMs int) {
	hold := time.Duration(holdMs) * time.Millisecond
	_, err := c.page.Evaluate(`([t, ms]) => {
		let c = document.getElementById('__cap');
		if (!c) {
			c = document.createElement('div');
			c.id = '__cap';
			c.style.cssText = 'position:fixed;left:50%;bottom:22px;transform:translateX(-50%);width:max-content;max-width:88%;' +
				'background:rgba(17,24,39,.93);color:#fff;font:600 19px/1.4 system-ui,sans-serif;padding:12px 20px 14px;' +
				'border-radius:10px;z-index:99999;text-align:center;box-shadow:0 6px 24px rgba(0,0,0,.25);pointer-events:none;overflow:hidden';
			c.innerHTML = '<span></span><i style="position:absolute;left:0;bottom:0;height:3px;background:#60a5fa;width:0"></i>';
			document.body.appendChild(c);
		}
		c.firstChild.textContent = t;
		c.style.display = t ? 'block' : 'none';
		const bar = c.lastChild;
		bar.style.animation = 'none'; void bar.offsetWidth;
		bar.style.animation = ms > 0 ? '__bar ' + ms + 'ms linear forwards' : 'none';
	}`, []any{text, hold.Milliseconds()})
	must(err)
	time.Sleep(hold)
}

func (c *clip) hideCaption() { c.caption("", 0) }

func (c *clip) shot(name string, full bool) {
	c.hideCaption()
	_, err := c.page.Screenshot(playwright.PageScreenshotOptions{
		Path: playwright.String(filepath.Join(shotsDir, name+".png")), FullPage: playwright.Bool(full),
	})
	must(err)
}

func (c *clip) btn(name string) playwright.Locator {
	return c.page.GetByRole(*playwright.AriaRoleButton, playwright.PageGetByRoleOptions{Name: name, Exact: playwright.Bool(true)})
}

func (c *clip) qty(product string) playwright.Locator {
	return c.page.GetByRole(*playwright.AriaRoleTextbox, playwright.PageGetByRoleOptions{Name: "Quantity of " + product})
}

func (c *clip) click(l playwright.Locator) {
	must(l.Click())
	time.Sleep(400 * time.Millisecond)
}

// typeSlowly types like a person so viewers can follow along.
func typeSlowly(l playwright.Locator, s string) {
	must(l.Click())
	must(l.PressSequentially(s, playwright.LocatorPressSequentiallyOptions{Delay: playwright.Float(110)}))
}

func (c *clip) finish() {
	close(c.stop)
	<-c.done
	must(c.ctx.Close())
}

func main() {
	must(os.MkdirAll(shotsDir, 0o755))
	must(os.RemoveAll(rawDir))
	must(os.MkdirAll(rawDir, 0o755))

	svc := checkout.NewService(1500 * time.Millisecond) // slow enough to see "Placing order…"
	srv := httptest.NewServer(web.NewHandler(svc, slog.New(slog.NewTextHandler(io.Discard, nil))))
	defer srv.Close()

	pw, err := playwright.Run()
	must(err)
	defer pw.Stop()
	browser, err := pw.Chromium.Launch()
	must(err)
	defer browser.Close()

	// ---------------- Clip 1: desktop customer journey ----------------
	a := newClip(browser, srv.URL, "1-desktop", 1280, 800, false)
	a.goTo("/")
	must(a.page.GetByText("Add to cart").First().WaitFor())
	a.caption("Mini Checkout: a Go backend serving a plain HTML/JS page. Prices come from the Go API.", 4000)
	a.shot("01-empty", false)

	a.caption("Add both products to the cart.", 0)
	a.click(a.btn("Add A5 Dotted Notebook to cart"))
	a.click(a.btn("Add LED Desk Lamp to cart"))
	a.caption("Line totals and the order total update straight away: ₹149.75 + ₹1,299.00 = ₹1,448.75", 3500)

	q := a.qty("A5 Dotted Notebook")
	a.caption("Type a quantity of 3: the totals update as you type.", 0)
	must(q.Fill(""))
	typeSlowly(q, "3")
	a.caption("3 × ₹149.75 = ₹449.25 exactly. Money is kept in integer paise, so there are no float rounding errors.", 4500)
	a.shot("02-cart-totals", false)

	a.caption("Now an invalid quantity: 0", 0)
	must(q.Fill(""))
	typeSlowly(q, "0")
	a.caption("Clear message, the input is kept, totals show “—” instead of a wrong number, and checkout is blocked.", 4500)
	a.shot("03-invalid-quantity", false)
	must(q.Fill(""))
	typeSlowly(q, "3")
	time.Sleep(600 * time.Millisecond)

	a.caption("Place order with the name left blank…", 1500)
	a.click(a.page.Locator("#place-order"))
	a.caption("Blocked with a clear message. The field is highlighted and focused, and the cart is kept.", 4000)
	a.shot("04-missing-name", false)

	a.caption("Enter a name, then double-click Place order quickly…", 0)
	typeSlowly(a.page.GetByLabel("Full name"), "Asha Verma")
	time.Sleep(700 * time.Millisecond)
	must(a.page.Locator("#place-order").Dblclick())
	time.Sleep(250 * time.Millisecond)
	a.shot("05-placing-order", false)
	a.caption("The button locks at once (“Placing order…”). The second click does nothing.", 0)
	must(a.page.GetByRole(*playwright.AriaRoleHeading, playwright.PageGetByRoleOptions{Name: "Order confirmed"}).WaitFor())
	time.Sleep(800 * time.Millisecond)
	a.caption("One confirmation with a readable reference. The server also deduplicates by idempotency key.", 4500)
	a.shot("06-confirmation", false)
	a.finish()

	// ---------------- Clip 2: phone layout ----------------
	m := newClip(browser, srv.URL, "2-mobile", 412, 800, true)
	m.goTo("/")
	must(m.page.GetByText("Add to cart").First().WaitFor())
	m.click(m.btn("Add A5 Dotted Notebook to cart"))
	m.click(m.btn("Add LED Desk Lamp to cart"))
	m.caption("On a phone each cart line becomes a card, so Remove is never off-screen (DEF-03 fix).", 4500)
	m.shot("07-mobile", true)
	m.click(m.btn("Remove LED Desk Lamp"))
	m.caption("Remove works with one tap.", 2000)
	m.finish()

	// ---------------- Clip 3: test report ----------------
	if abs, err := filepath.Abs("docs/report/index.html"); err == nil {
		if _, err := os.Stat(abs); err == nil {
			r := newClip(browser, "", "3-report", 1280, 800, false)
			r.goTo("file://" + abs)
			r.caption("Test report from a real run: Go unit tests plus browser tests on Chromium, Firefox, WebKit and mobile.", 5000)
			r.shot("08-test-report", false)
			_, err = r.page.Evaluate(`() => window.scrollTo({ top: 520, behavior: 'smooth' })`)
			must(err)
			r.caption("Every scenario passes on every engine, with retries turned off.", 4000)
			r.finish()
		}
	}
	log.Printf("clips in %s, screenshots in %s", rawDir, shotsDir)
}
