// Package checkout holds the pricing, validation and order rules for the mini store.
// It has no HTTP or browser code so the business rules can be unit tested directly.
package checkout

// Paise is an amount of money in paise (1 rupee = 100 paise).
// Money is always an integer so totals are exact: 3 x ₹149.75 is 44925 paise,
// never 449.24999...
type Paise int64

// Product is an item the store sells. All products are synthetic test data.
type Product struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Price Paise  `json:"pricePaise"`
}

// Quantity limits per cart line.
const (
	MinQty = 1
	MaxQty = 10
)

// Catalog is the fixed list of products. The order matters: it is the display order.
var Catalog = []Product{
	{ID: "notebook", Name: "A5 Dotted Notebook", Price: 14975}, // ₹149.75 — non-round on purpose
	{ID: "lamp", Name: "LED Desk Lamp", Price: 129900},         // ₹1,299.00 — exercises the thousands separator
}

// FindProduct returns the product with the given ID.
func FindProduct(id string) (Product, bool) {
	for _, p := range Catalog {
		if p.ID == id {
			return p, true
		}
	}
	return Product{}, false
}
