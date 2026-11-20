package checkout

import (
	"fmt"
	"strconv"
)

// FormatINR formats paise the way an Indian store shows prices: ₹1,299.00, ₹1,00,000.00.
// It uses Indian digit grouping (last three digits, then groups of two), which is also
// what the browser's Intl.NumberFormat("en-IN") produces, so server and UI always agree.
func FormatINR(p Paise) string {
	sign := ""
	if p < 0 {
		sign = "-"
		p = -p
	}
	rupees := int64(p) / 100
	paise := int64(p) % 100
	return fmt.Sprintf("%s₹%s.%02d", sign, groupIndian(strconv.FormatInt(rupees, 10)), paise)
}

func groupIndian(digits string) string {
	if len(digits) <= 3 {
		return digits
	}
	head, tail := digits[:len(digits)-3], digits[len(digits)-3:]
	out := ""
	for len(head) > 2 {
		out = "," + head[len(head)-2:] + out
		head = head[:len(head)-2]
	}
	return head + out + "," + tail
}
