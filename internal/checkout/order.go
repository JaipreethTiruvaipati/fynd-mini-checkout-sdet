package checkout

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// LineItem is one product and quantity in an order request.
type LineItem struct {
	ProductID string `json:"productId"`
	Qty       int    `json:"qty"`
}

// OrderRequest is what the browser sends when the customer clicks Place order.
type OrderRequest struct {
	// IdempotencyKey identifies one checkout attempt. Sending the same key again
	// (double-click, network retry) returns the original order instead of a new one.
	IdempotencyKey string     `json:"idempotencyKey"`
	CustomerName   string     `json:"customerName"`
	Items          []LineItem `json:"items"`
	// ExpectedTotal is the total the customer saw. If the server's total differs
	// (for example prices changed) the order is refused rather than charging a surprise amount.
	ExpectedTotal Paise `json:"expectedTotalPaise"`
}

// OrderLine is a priced line on a placed order.
type OrderLine struct {
	ProductID string `json:"productId"`
	Name      string `json:"name"`
	Qty       int    `json:"qty"`
	UnitPrice Paise  `json:"unitPricePaise"`
	LineTotal Paise  `json:"lineTotalPaise"`
}

// Order is a placed order.
type Order struct {
	Reference    string      `json:"reference"`
	CustomerName string      `json:"customerName"`
	Lines        []OrderLine `json:"lines"`
	Total        Paise       `json:"totalPaise"`
	TotalText    string      `json:"totalText"`
	CreatedAt    time.Time   `json:"createdAt"`
}

// FieldError is a validation problem the customer can fix.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// Customer-facing messages. The browser shows the same text, and the tests assert it.
const (
	MsgEmptyCart   = "Your cart is empty. Add at least one item before placing your order."
	MsgNameMissing = "Please enter your full name."
	MsgNameTooLong = "Name must be 80 characters or fewer."
	MsgBadQty      = "Enter a whole number from 1 to 10."
	MsgUnknownItem = "This product is no longer available."
	MsgDuplicate   = "Each product can appear only once in the cart."
	MsgBadKey      = "Missing or invalid checkout session. Please refresh the page."
)

const maxNameLen = 80

// ValidationError is returned when the request has customer-fixable problems.
type ValidationError struct{ Errors []FieldError }

func (e *ValidationError) Error() string {
	return fmt.Sprintf("invalid order: %d problem(s)", len(e.Errors))
}

var (
	// ErrPriceChanged means the total the customer saw is not the real total.
	ErrPriceChanged = errors.New("order total has changed; please review your cart")
	// ErrKeyReused means an idempotency key was reused for a different order.
	ErrKeyReused = errors.New("idempotency key was already used for a different order")
)

// Price validates the request and returns the priced lines and total.
// It never trusts prices from the browser: unit prices come from the Catalog.
func Price(req OrderRequest) ([]OrderLine, Paise, error) {
	var errs []FieldError
	add := func(field, msg string) { errs = append(errs, FieldError{Field: field, Message: msg}) }

	if k := req.IdempotencyKey; len(k) < 8 || len(k) > 100 {
		add("idempotencyKey", MsgBadKey)
	}
	name := strings.TrimSpace(req.CustomerName)
	switch {
	case name == "":
		add("customerName", MsgNameMissing)
	case utf8.RuneCountInString(name) > maxNameLen:
		add("customerName", MsgNameTooLong)
	}
	if len(req.Items) == 0 {
		add("items", MsgEmptyCart)
	}

	var lines []OrderLine
	var total Paise
	seen := map[string]bool{}
	for i, it := range req.Items {
		field := fmt.Sprintf("items[%d]", i)
		p, ok := FindProduct(it.ProductID)
		switch {
		case !ok:
			add(field, MsgUnknownItem)
			continue
		case seen[it.ProductID]:
			add(field, MsgDuplicate)
			continue
		case it.Qty < MinQty || it.Qty > MaxQty:
			add(field, MsgBadQty)
			continue
		}
		seen[it.ProductID] = true
		lt := p.Price * Paise(it.Qty)
		lines = append(lines, OrderLine{ProductID: p.ID, Name: p.Name, Qty: it.Qty, UnitPrice: p.Price, LineTotal: lt})
		total += lt
	}

	if len(errs) > 0 {
		return nil, 0, &ValidationError{Errors: errs}
	}
	if req.ExpectedTotal != total {
		return nil, 0, ErrPriceChanged
	}
	return lines, total, nil
}

// Service places orders. It is safe for concurrent use.
//
// Duplicate protection: the first request for an idempotency key creates the order;
// any concurrent or later request with the same key waits for and returns that same
// order. This holds even if the browser's own guard is bypassed (two tabs sharing a
// key, a retried request after a timeout, a script).
type Service struct {
	// ProcessingDelay simulates payment/inventory work, so a double-click really does
	// race against an in-flight order. Zero in unit tests.
	ProcessingDelay time.Duration
	Now             func() time.Time

	mu      sync.Mutex
	byKey   map[string]*attempt
	orders  []Order
	refs    map[string]bool
	reqSeen int
}

type attempt struct {
	fingerprint [32]byte
	done        chan struct{}
	order       Order
	err         error
}

// NewService returns an empty order service.
func NewService(delay time.Duration) *Service {
	return &Service{ProcessingDelay: delay, Now: time.Now, byKey: map[string]*attempt{}, refs: map[string]bool{}}
}

// PlaceOrder creates an order, or returns the existing one for a repeated idempotency key.
// created reports whether this call created a new order.
func (s *Service) PlaceOrder(ctx context.Context, req OrderRequest) (order Order, created bool, err error) {
	s.mu.Lock()
	s.reqSeen++
	s.mu.Unlock()

	lines, total, err := Price(req)
	if err != nil {
		return Order{}, false, err
	}
	fp := fingerprint(req)

	s.mu.Lock()
	if a, ok := s.byKey[req.IdempotencyKey]; ok {
		s.mu.Unlock()
		if a.fingerprint != fp {
			return Order{}, false, ErrKeyReused
		}
		select {
		case <-a.done:
			return a.order, false, a.err
		case <-ctx.Done():
			return Order{}, false, ctx.Err()
		}
	}
	a := &attempt{fingerprint: fp, done: make(chan struct{})}
	s.byKey[req.IdempotencyKey] = a
	s.mu.Unlock()

	if s.ProcessingDelay > 0 {
		select {
		case <-time.After(s.ProcessingDelay):
		case <-ctx.Done():
			// The attempt never completed: forget the key so a retry can succeed.
			s.mu.Lock()
			delete(s.byKey, req.IdempotencyKey)
			s.mu.Unlock()
			a.err = ctx.Err()
			close(a.done)
			return Order{}, false, ctx.Err()
		}
	}

	s.mu.Lock()
	a.order = Order{
		Reference:    s.newReferenceLocked(),
		CustomerName: strings.TrimSpace(req.CustomerName),
		Lines:        lines,
		Total:        total,
		TotalText:    FormatINR(total),
		CreatedAt:    s.Now().UTC(),
	}
	s.orders = append(s.orders, a.order)
	s.mu.Unlock()
	close(a.done)
	return a.order, true, nil
}

// Orders returns a copy of all placed orders (used by tests and for inspection).
func (s *Service) Orders() []Order {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Order(nil), s.orders...)
}

// RequestsSeen returns how many PlaceOrder calls were made, including duplicates and invalid ones.
func (s *Service) RequestsSeen() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reqSeen
}

// Reference alphabet leaves out 0/O and 1/I so a customer can read it out on a support call.
const refAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func (s *Service) newReferenceLocked() string {
	for {
		b := make([]byte, 6)
		if _, err := rand.Read(b); err != nil {
			panic(err) // crypto/rand never fails on supported platforms
		}
		for i := range b {
			b[i] = refAlphabet[int(b[i])%len(refAlphabet)]
		}
		ref := "ORD-" + string(b)
		if !s.refs[ref] {
			s.refs[ref] = true
			return ref
		}
	}
}

func fingerprint(req OrderRequest) [32]byte {
	var sb strings.Builder
	sb.WriteString(strings.TrimSpace(req.CustomerName))
	for _, it := range req.Items {
		fmt.Fprintf(&sb, "|%s:%d", it.ProductID, it.Qty)
	}
	return sha256.Sum256([]byte(sb.String()))
}
