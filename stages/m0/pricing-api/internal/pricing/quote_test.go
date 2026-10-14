package pricing

import (
	"errors"
	"testing"
)

func TestCalculate(t *testing.T) {
	tests := []struct {
		name   string
		amount int64
		tier   string
		coupon string
		want   Quote
	}{
		{"standard", 20_000, "standard", "", Quote{20_000, 0, 0, 20_000}},
		{"vip", 20_000, "vip", "", Quote{20_000, 2_000, 0, 18_000}},
		{"coupon", 20_000, "standard", "SAVE100", Quote{20_000, 0, 10_000, 10_000}},
		{"vip then coupon", 20_000, "vip", "SAVE100", Quote{20_000, 2_000, 10_000, 8_000}},
		{"floor to cent", 101, "vip", "", Quote{101, 11, 0, 90}},
		{"clamp coupon", 100, "vip", "SAVE100", Quote{100, 10, 90, 0}},
		{"zero", 0, "vip", "SAVE100", Quote{}},
		{"maximum", MaxAmountCents, "vip", "SAVE100", Quote{MaxAmountCents, 100_000_000_000, 10_000, 899_999_990_000}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := Calculate(QuoteRequest{&test.amount, test.tier, test.coupon})
			if err != nil || got != test.want {
				t.Fatalf("Calculate() = %+v, %v; want %+v", got, err, test.want)
			}
		})
	}
}

func TestCalculateRejectsInvalidInput(t *testing.T) {
	negative, excessive, valid := int64(-1), MaxAmountCents+1, int64(100)
	for _, request := range []QuoteRequest{
		{nil, "standard", ""}, {&negative, "standard", ""}, {&excessive, "standard", ""},
		{&valid, "", ""}, {&valid, "admin", ""}, {&valid, "VIP", ""}, {&valid, "vip", "invalid"},
	} {
		if _, err := Calculate(request); !errors.Is(err, ErrInvalidQuote) {
			t.Errorf("Calculate(%+v) error = %v; want ErrInvalidQuote", request, err)
		}
	}
}

var benchmarkQuote Quote

func BenchmarkCalculate(b *testing.B) {
	amount := int64(20_000)
	request := QuoteRequest{&amount, "vip", "SAVE100"}
	var quote Quote
	for b.Loop() {
		var err error
		quote, err = Calculate(request)
		if err != nil {
			b.Fatal(err)
		}
	}
	benchmarkQuote = quote
}
