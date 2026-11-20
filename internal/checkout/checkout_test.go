package checkout

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestFormatINR(t *testing.T) {
	tests := []struct {
		in   Paise
		want string
	}{
		{0, "₹0.00"},
		{5, "₹0.05"},
		{14975, "₹149.75"},
		{44925, "₹449.25"}, // 3 x 149.75: the classic float trap (449.24999...)
		{129900, "₹1,299.00"},
		{174825, "₹1,748.25"},
		{10000000, "₹1,00,000.00"},     // Indian grouping: lakh
		{1234567890, "₹1,23,45,678.90"}, // crore
		{-14975, "-₹149.75"},
	}
	for _, tt := range tests {
		if got := FormatINR(tt.in); got != tt.want {
			t.Errorf("FormatINR(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func validReq() OrderRequest {
	return OrderRequest{
		IdempotencyKey: "key-12345678",
		CustomerName:   "Asha Verma",
		Items:          []LineItem{{ProductID: "notebook", Qty: 3}, {ProductID: "lamp", Qty: 1}},
		ExpectedTotal:  174825,
	}
}

func TestPrice_ValidOrderIsPricedFromCatalog(t *testing.T) {
	lines, total, err := Price(validReq())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 174825 {
		t.Errorf("total = %d, want 174825 (₹1,748.25)", total)
	}
	if len(lines) != 2 || lines[0].LineTotal != 44925 || lines[1].LineTotal != 129900 {
		t.Errorf("unexpected lines: %+v", lines)
	}
}

func TestPrice_Validation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*OrderRequest)
		field  string
		msg    string
	}{
		{"empty cart", func(r *OrderRequest) { r.Items = nil }, "items", MsgEmptyCart},
		{"blank name", func(r *OrderRequest) { r.CustomerName = "" }, "customerName", MsgNameMissing},
		{"whitespace name", func(r *OrderRequest) { r.CustomerName = " \t\n " }, "customerName", MsgNameMissing},
		{"name too long", func(r *OrderRequest) { r.CustomerName = strings.Repeat("a", 81) }, "customerName", MsgNameTooLong},
		{"qty zero", func(r *OrderRequest) { r.Items[0].Qty = 0 }, "items[0]", MsgBadQty},
		{"qty negative", func(r *OrderRequest) { r.Items[0].Qty = -1 }, "items[0]", MsgBadQty},
		{"qty above max", func(r *OrderRequest) { r.Items[0].Qty = 11 }, "items[0]", MsgBadQty},
		{"unknown product", func(r *OrderRequest) { r.Items[1].ProductID = "phone" }, "items[1]", MsgUnknownItem},
		{"duplicate product", func(r *OrderRequest) { r.Items[1].ProductID = "notebook" }, "items[1]", MsgDuplicate},
		{"missing idempotency key", func(r *OrderRequest) { r.IdempotencyKey = "" }, "idempotencyKey", MsgBadKey},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := validReq()
			tt.mutate(&req)
			_, _, err := Price(req)
			var verr *ValidationError
			if !errors.As(err, &verr) {
				t.Fatalf("want ValidationError, got %v", err)
			}
			for _, fe := range verr.Errors {
				if fe.Field == tt.field && fe.Message == tt.msg {
					return
				}
			}
			t.Errorf("want error {%s: %q}, got %+v", tt.field, tt.msg, verr.Errors)
		})
	}
}

func TestPrice_BoundaryQuantitiesAreAccepted(t *testing.T) {
	for _, q := range []int{MinQty, MaxQty} {
		req := validReq()
		req.Items = []LineItem{{ProductID: "lamp", Qty: q}}
		req.ExpectedTotal = 129900 * Paise(q)
		if _, _, err := Price(req); err != nil {
			t.Errorf("qty %d rejected: %v", q, err)
		}
	}
}

func TestPrice_RejectsTotalTheCustomerDidNotSee(t *testing.T) {
	req := validReq()
	req.ExpectedTotal = 174824 // off by one paisa
	if _, _, err := Price(req); !errors.Is(err, ErrPriceChanged) {
		t.Fatalf("want ErrPriceChanged, got %v", err)
	}
}

var refPattern = regexp.MustCompile(`^ORD-[A-HJ-NP-Z2-9]{6}$`)

func TestPlaceOrder_CreatesOrderWithReadableReference(t *testing.T) {
	svc := NewService(0)
	o, created, err := svc.PlaceOrder(context.Background(), validReq())
	if err != nil || !created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	if !refPattern.MatchString(o.Reference) {
		t.Errorf("reference %q does not match %s", o.Reference, refPattern)
	}
	if o.TotalText != "₹1,748.25" || o.CustomerName != "Asha Verma" {
		t.Errorf("unexpected order: %+v", o)
	}
}

func TestPlaceOrder_SameKeyReturnsSameOrder(t *testing.T) {
	svc := NewService(0)
	first, _, _ := svc.PlaceOrder(context.Background(), validReq())
	second, created, err := svc.PlaceOrder(context.Background(), validReq())
	if err != nil || created {
		t.Fatalf("second call: created=%v err=%v, want replay", created, err)
	}
	if second.Reference != first.Reference {
		t.Errorf("replay returned %s, want %s", second.Reference, first.Reference)
	}
	if n := len(svc.Orders()); n != 1 {
		t.Errorf("orders = %d, want 1", n)
	}
}

func TestPlaceOrder_DifferentKeysCreateDifferentOrders(t *testing.T) {
	svc := NewService(0)
	a, b := validReq(), validReq()
	b.IdempotencyKey = "key-87654321"
	o1, _, _ := svc.PlaceOrder(context.Background(), a)
	o2, _, _ := svc.PlaceOrder(context.Background(), b)
	if o1.Reference == o2.Reference || len(svc.Orders()) != 2 {
		t.Errorf("want two distinct orders, got %s and %s", o1.Reference, o2.Reference)
	}
}

func TestPlaceOrder_KeyReusedForDifferentCartIsRejected(t *testing.T) {
	svc := NewService(0)
	_, _, _ = svc.PlaceOrder(context.Background(), validReq())
	other := validReq()
	other.Items = []LineItem{{ProductID: "lamp", Qty: 1}}
	other.ExpectedTotal = 129900
	if _, _, err := svc.PlaceOrder(context.Background(), other); !errors.Is(err, ErrKeyReused) {
		t.Fatalf("want ErrKeyReused, got %v", err)
	}
}

// The double-click race at the server: 50 identical requests arrive while the first is
// still processing. Exactly one order must exist and everyone must see the same reference.
// Run with -race.
func TestPlaceOrder_ConcurrentDuplicatesCreateExactlyOneOrder(t *testing.T) {
	svc := NewService(20 * time.Millisecond)
	const n = 50
	refs := make([]string, n)
	createdCount := 0
	var mu sync.Mutex
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			o, created, err := svc.PlaceOrder(context.Background(), validReq())
			if err != nil {
				t.Errorf("request %d: %v", i, err)
				return
			}
			mu.Lock()
			refs[i] = o.Reference
			if created {
				createdCount++
			}
			mu.Unlock()
		}()
	}
	close(start)
	wg.Wait()

	if got := len(svc.Orders()); got != 1 {
		t.Fatalf("orders created = %d, want 1", got)
	}
	if createdCount != 1 {
		t.Errorf("created=true returned %d times, want 1", createdCount)
	}
	for i, r := range refs {
		if r != refs[0] {
			t.Errorf("request %d got %s, want %s", i, r, refs[0])
		}
	}
}

func TestPlaceOrder_CancelledAttemptCanBeRetried(t *testing.T) {
	svc := NewService(time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := svc.PlaceOrder(ctx, validReq()); !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
	svc.ProcessingDelay = 0
	if _, created, err := svc.PlaceOrder(context.Background(), validReq()); err != nil || !created {
		t.Fatalf("retry: created=%v err=%v", created, err)
	}
}
