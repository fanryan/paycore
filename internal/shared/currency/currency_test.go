package currency

import "testing"

func TestNormalizeCurrency(t *testing.T) {
	got := NormalizeCurrency(" usd ")

	if got != "USD" {
		t.Fatalf("expected USD, got %q", got)
	}
}

func TestIsValidCurrency(t *testing.T) {
	tests := []struct {
		name     string
		currency string
		want     bool
	}{
		{name: "valid uppercase", currency: "USD", want: true},
		{name: "valid lowercase after normalization", currency: "sgd", want: true},
		{name: "too short", currency: "US", want: false},
		{name: "too long", currency: "USDT", want: false},
		{name: "number", currency: "U5D", want: false},
		{name: "blank", currency: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsValidCurrency(tt.currency)

			if got != tt.want {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
		})
	}
}
