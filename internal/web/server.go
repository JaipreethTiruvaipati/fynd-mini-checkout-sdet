// Package web exposes the checkout over HTTP and serves the single-page UI.
package web

import (
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/JaipreethTiruvaipati/fynd-mini-checkout-sdet/internal/checkout"
)

//go:embed static
var staticFiles embed.FS

const maxBodyBytes = 16 << 10

// NewHandler returns the HTTP handler for the app.
//
//	GET  /api/products  -> catalogue with prices in paise
//	POST /api/orders    -> place an order (idempotent per idempotencyKey)
//	GET  /              -> the checkout page
func NewHandler(svc *checkout.Service, log *slog.Logger) http.Handler {
	static, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/products", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"products": checkout.Catalog})
	})
	mux.HandleFunc("POST /api/orders", placeOrder(svc, log))
	// Without this, "GET /" would catch GET /api/orders and answer 404 instead of 405.
	// (Other methods such as PUT get an automatic 405 from ServeMux.)
	mux.HandleFunc("GET /api/orders", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Allow", http.MethodPost)
		writeJSON(w, http.StatusMethodNotAllowed, errorBody("method_not_allowed", "Use POST to place an order."))
	})
	mux.Handle("GET /", http.FileServerFS(static))
	return securityHeaders(mux)
}

func placeOrder(svc *checkout.Service, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		var req checkout.OrderRequest
		if err := dec.Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorBody("bad_request", "Request body is not valid JSON for an order."))
			return
		}

		order, created, err := svc.PlaceOrder(r.Context(), req)
		var verr *checkout.ValidationError
		switch {
		case errors.As(err, &verr):
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "validation_failed", "errors": verr.Errors})
		case errors.Is(err, checkout.ErrPriceChanged):
			writeJSON(w, http.StatusConflict, errorBody("price_changed", "Prices have changed since you loaded the page. Please review your cart."))
		case errors.Is(err, checkout.ErrKeyReused):
			writeJSON(w, http.StatusConflict, errorBody("idempotency_key_reused", "This checkout session was already used for a different order."))
		case err != nil:
			log.Warn("place order failed", "err", err)
			writeJSON(w, http.StatusServiceUnavailable, errorBody("unavailable", "We couldn't place your order. Please try again."))
		case created:
			log.Info("order placed", "reference", order.Reference, "total", order.TotalText)
			writeJSON(w, http.StatusCreated, map[string]any{"order": order})
		default:
			// Same idempotency key again: return the original order, create nothing.
			log.Info("duplicate submit returned existing order", "reference", order.Reference)
			w.Header().Set("Idempotent-Replayed", "true")
			writeJSON(w, http.StatusOK, map[string]any{"order": order})
		}
	}
}

func errorBody(code, msg string) map[string]any {
	return map[string]any{"error": code, "message": msg}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; frame-ancestors 'none'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

// RequestTimeout bounds how long one API request may take.
const RequestTimeout = 10 * time.Second
