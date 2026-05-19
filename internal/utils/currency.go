package utils

var ValidCurrencies = map[string]bool{
	"KES": true, "USD": true, "EUR": true, "GBP": true,
	"TZS": true, "UGX": true, "NGN": true, "ZAR": true,
	"RWF": true, "BWP": true, "GHS": true, "ETB": true,
}

const DefaultCurrency = "KES"

func IsValidCurrency(currency string) bool {
	return ValidCurrencies[currency]
}

var CurrencyNames = map[string]string{
	"KES": "KES - Kenyan Shilling",
	"USD": "USD - US Dollar",
	"EUR": "EUR - Euro",
	"GBP": "GBP - British Pound",
	"TZS": "TZS - Tanzanian Shilling",
	"UGX": "UGX - Ugandan Shilling",
	"NGN": "NGN - Nigerian Naira",
	"ZAR": "ZAR - South African Rand",
	"RWF": "RWF - Rwandan Franc",
	"BWP": "BWP - Botswana Pula",
	"GHS": "GHS - Ghanaian Cedi",
	"ETB": "ETB - Ethiopian Birr",
}