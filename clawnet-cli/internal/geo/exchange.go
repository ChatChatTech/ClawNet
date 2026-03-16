package geo

import (
	"fmt"
	"strings"
)

// CurrencyInfo describes a local currency and its Shell exchange rate.
type CurrencyInfo struct {
	Code   string  // ISO 4217 code, e.g. "CNY"
	Symbol string  // display prefix, e.g. "¥"
	Rate   float64 // 1 Shell ≈ Rate units of this currency
}

// Format returns a human-readable string like "¥1,250 CNY" for the given Shell amount.
func (c *CurrencyInfo) Format(shells int64) string {
	v := float64(shells) * c.Rate
	// For whole numbers, format with commas
	if v == float64(int64(v)) {
		return fmt.Sprintf("%s%s %s", c.Symbol, commaInt(int64(v)), c.Code)
	}
	return fmt.Sprintf("%s%.2f %s", c.Symbol, v, c.Code)
}

// commaInt formats an integer with comma separators.
func commaInt(n int64) string {
	s := fmt.Sprintf("%d", n)
	if n < 0 {
		return "-" + addCommas(s[1:])
	}
	return addCommas(s)
}

func addCommas(s string) string {
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	rem := len(s) % 3
	if rem > 0 {
		b.WriteString(s[:rem])
		b.WriteByte(',')
	}
	for i := rem; i < len(s); i += 3 {
		if i > rem {
			b.WriteByte(',')
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
}

// currencies maps ISO 4217 code → CurrencyInfo.
// Rates are hardcoded based on 2026-03-16 exchange rates (1 Shell ≈ ¥1 CNY).
var currencies = map[string]*CurrencyInfo{
	"CNY": {Code: "CNY", Symbol: "¥", Rate: 1},
	"USD": {Code: "USD", Symbol: "$", Rate: 0.14},
	"EUR": {Code: "EUR", Symbol: "€", Rate: 0.13},
	"GBP": {Code: "GBP", Symbol: "£", Rate: 0.11},
	"JPY": {Code: "JPY", Symbol: "¥", Rate: 21},
	"KRW": {Code: "KRW", Symbol: "₩", Rate: 27},
	"HKD": {Code: "HKD", Symbol: "HK$", Rate: 1.09},
	"TWD": {Code: "TWD", Symbol: "NT$", Rate: 4.52},
	"SGD": {Code: "SGD", Symbol: "S$", Rate: 0.19},
	"INR": {Code: "INR", Symbol: "₹", Rate: 11.70},
	"RUB": {Code: "RUB", Symbol: "₽", Rate: 12.80},
	"BRL": {Code: "BRL", Symbol: "R$", Rate: 0.80},
	"CAD": {Code: "CAD", Symbol: "C$", Rate: 0.19},
	"AUD": {Code: "AUD", Symbol: "A$", Rate: 0.21},
	"MXN": {Code: "MXN", Symbol: "MX$", Rate: 2.80},
	"THB": {Code: "THB", Symbol: "฿", Rate: 4.80},
	"VND": {Code: "VND", Symbol: "₫", Rate: 3500},
	"MYR": {Code: "MYR", Symbol: "RM", Rate: 0.62},
}

// countryToCurrency maps ISO 3166-1 alpha-2 country codes to currency codes.
var countryToCurrency = map[string]string{
	"CN": "CNY",
	"US": "USD", "EC": "USD", "PA": "USD", "SV": "USD", "PR": "USD",
	"DE": "EUR", "FR": "EUR", "IT": "EUR", "ES": "EUR", "NL": "EUR",
	"BE": "EUR", "AT": "EUR", "PT": "EUR", "FI": "EUR", "IE": "EUR",
	"GR": "EUR", "LU": "EUR", "SK": "EUR", "SI": "EUR", "EE": "EUR",
	"LV": "EUR", "LT": "EUR", "MT": "EUR", "CY": "EUR", "HR": "EUR",
	"GB": "GBP",
	"JP": "JPY",
	"KR": "KRW",
	"HK": "HKD",
	"TW": "TWD",
	"SG": "SGD",
	"IN": "INR",
	"RU": "RUB",
	"BR": "BRL",
	"CA": "CAD",
	"AU": "AUD",
	"MX": "MXN",
	"TH": "THB",
	"VN": "VND",
	"MY": "MYR",
}

// defaultCurrency is returned when a country is not in the map.
var defaultCurrency = currencies["CNY"]

// CurrencyForCountry returns the CurrencyInfo for the given ISO country code.
// Falls back to CNY if the country is unknown.
func CurrencyForCountry(country string) *CurrencyInfo {
	if code, ok := countryToCurrency[country]; ok {
		if c, ok := currencies[code]; ok {
			return c
		}
	}
	return defaultCurrency
}
