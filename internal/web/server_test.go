package web

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JaipreethTiruvaipati/fynd-mini-checkout-sdet/internal/checkout"
)

func newTestServer(t *testing.T) (*httptest.Server, *checkout.Service) {
	t.Helper()
	svc := checkout.NewService(0)
	srv := httptest.NewServer(NewHandler(svc, slog.New(slog.NewTextHandler(io.Discard, nil))))
	t.Cleanup(srv.Close)
	return srv, svc
}

func post(t *testing.T, url, body string) (*http.Response, map[string]any) {
	t.Helper()
	res, err := http.Post(url+"/api/orders", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res, out
}

const validBody = `{"idempotencyKey":"k-1234567890","customerName":"Asha Verma",
 "items":[{"productId":"notebook","qty":3}],"expectedTotalPaise":44925}`

func TestProducts(t *testing.T) {
	srv, _ := newTestServer(t)
	res, err := http.Get(srv.URL + "/api/products")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var body struct{ Products []checkout.Product }
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 200 || len(body.Products) != 2 || body.Products[0].Price != 14975 {
		t.Fatalf("status=%d products=%+v", res.StatusCode, body.Products)
	}
}

func TestPlaceOrder_CreatedThenReplayed(t *testing.T) {
	srv, svc := newTestServer(t)

	res1, b1 := post(t, srv.URL, validBody)
	if res1.StatusCode != http.StatusCreated {
		t.Fatalf("first: status %d, body %v", res1.StatusCode, b1)
	}
	res2, b2 := post(t, srv.URL, validBody)
	if res2.StatusCode != http.StatusOK || res2.Header.Get("Idempotent-Replayed") != "true" {
		t.Fatalf("replay: status %d, header %q", res2.StatusCode, res2.Header.Get("Idempotent-Replayed"))
	}
	ref1 := b1["order"].(map[string]any)["reference"]
	ref2 := b2["order"].(map[string]any)["reference"]
	if ref1 != ref2 || len(svc.Orders()) != 1 {
		t.Fatalf("refs %v / %v, orders %d", ref1, ref2, len(svc.Orders()))
	}
}

func TestPlaceOrder_ErrorResponses(t *testing.T) {
	tests := []struct {
		name, body string
		status     int
		code       string
	}{
		{"validation", `{"idempotencyKey":"k-1234567890","customerName":" ","items":[],"expectedTotalPaise":0}`, 422, "validation_failed"},
		{"price changed", `{"idempotencyKey":"k-1234567890","customerName":"A","items":[{"productId":"lamp","qty":1}],"expectedTotalPaise":100}`, 409, "price_changed"},
		{"malformed json", `{"idempotencyKey":`, 400, "bad_request"},
		{"unknown field", `{"idempotencyKey":"k-1234567890","pricePaise":1}`, 400, "bad_request"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, svc := newTestServer(t)
			res, body := post(t, srv.URL, tt.body)
			if res.StatusCode != tt.status || body["error"] != tt.code {
				t.Fatalf("got %d %v, want %d %s", res.StatusCode, body["error"], tt.status, tt.code)
			}
			if len(svc.Orders()) != 0 {
				t.Fatal("an order was created for a rejected request")
			}
		})
	}
}

func TestStaticPageServedWithSecurityHeaders(t *testing.T) {
	srv, _ := newTestServer(t)
	res, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 || !strings.Contains(string(b), "<h1>Mini Checkout</h1>") {
		t.Fatalf("status %d", res.StatusCode)
	}
	if res.Header.Get("Content-Security-Policy") == "" || res.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Error("missing security headers")
	}
}

func TestOnlyPostAllowedOnOrders(t *testing.T) {
	srv, _ := newTestServer(t)
	res, err := http.Get(srv.URL + "/api/orders")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("GET /api/orders = %d, want 405", res.StatusCode)
	}
}
