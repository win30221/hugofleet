package pricing

import "errors"

// MaxAmountCents bounds the demo input and keeps all intermediate arithmetic safe.
const MaxAmountCents int64 = 1_000_000_000_000

var ErrInvalidQuote = errors.New("invalid quote")

type QuoteRequest struct {
	AmountCents  *int64 `json:"amount_cents"`
	CustomerTier string `json:"customer_tier"`
	CouponCode   string `json:"coupon_code"`
}

type Quote struct {
	OriginalAmountCents int64 `json:"original_amount_cents"`
	TierDiscountCents   int64 `json:"tier_discount_cents"`
	CouponDiscountCents int64 `json:"coupon_discount_cents"`
	FinalAmountCents    int64 `json:"final_amount_cents"`
}

func Calculate(request QuoteRequest) (Quote, error) {
	if request.AmountCents == nil || *request.AmountCents < 0 || *request.AmountCents > MaxAmountCents {
		return Quote{}, ErrInvalidQuote
	}
	if request.CustomerTier != "standard" && request.CustomerTier != "vip" {
		return Quote{}, ErrInvalidQuote
	}
	if request.CouponCode != "" && request.CouponCode != "SAVE100" {
		return Quote{}, ErrInvalidQuote
	}
	amount := *request.AmountCents
	final := amount
	if request.CustomerTier == "vip" {
		// Round the discounted price down to a whole cent before applying a coupon.
		final = amount * 9 / 10
	}
	quote := Quote{OriginalAmountCents: amount, TierDiscountCents: amount - final}
	if request.CouponCode == "SAVE100" {
		quote.CouponDiscountCents = min(final, 10_000)
		final -= quote.CouponDiscountCents
	}
	quote.FinalAmountCents = final
	return quote, nil
}
