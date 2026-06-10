package currency

import (
	"strings"
	"unicode"
)

func NormalizeCurrency(currency string) string {
	return strings.ToUpper(strings.TrimSpace(currency))
}

func IsValidCurrency(currency string) bool {
	currency = NormalizeCurrency(currency)

	if len(currency) != 3 {
		return false
	}

	for _, char := range currency {
		if !unicode.IsLetter(char) || !unicode.IsUpper(char) {
			return false
		}
	}

	return true
}
