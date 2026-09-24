package oblodai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
)

// Decimal is an amount as the API speaks it: a decimal string at the asset's own scale ("25",
// "10.000000"). It is a string type on purpose — USDT has 6 decimals, BTC 8 and ETH 18, and a
// float64 holds none of them exactly — so a float does not fit where an amount goes, and a JSON
// number decoded into one is refused with sdk.float_amount. It goes on the wire verbatim.
//
// Go will happily let you write a < b on two Decimals: do not. "9" < "10" is true as text and
// false as money. Order amounts with CompareAmounts, add them with AddAmounts.
type Decimal string

// String is the amount as written.
func (d Decimal) String() string { return string(d) }

// UnmarshalJSON accepts a JSON string (or null, which leaves d alone) and refuses a JSON number:
// by the time an amount is a binary float it may already be wrong.
func (d *Decimal) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if string(data) == "null" {
		return nil
	}
	if len(data) > 0 && data[0] != '"' {
		return newConfigError(CodeFloatAmount, fmt.Sprintf(
			"an amount must be a decimal string such as \"25.10\", not the JSON number %s: a float cannot hold money exactly",
			data), "")
	}
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	*d = Decimal(s)
	return nil
}

// Amount is what the money helpers take: a Decimal, or a plain string — the API sends amounts
// that may be empty ("" until a payment is paid) as strings.
type Amount interface{ ~string }

// Amounts are decimal strings; never parse them into a float64. These helpers compare and add at
// arbitrary precision, and give back a Decimal in the same shape the API speaks.
//
// An input that is not -?digits[.digits] (at most 64 characters, no empty integer part, no empty
// fraction after the dot, no exponent, no spaces) is refused with a ConfigError carrying
// sdk.bad_amount. Nothing here panics and nothing is silently coerced.

// MaxAmountLength bounds the length of an amount string these helpers accept.
const MaxAmountLength = 64

// CompareAmounts returns -1, 0 or 1 as a is less than, equal to or greater than b. Values of
// different scale compare correctly: "25" equals "25.000000".
func CompareAmounts[A, B Amount](a A, b B) (int, error) {
	x, y, err := alignedPair(Decimal(a), Decimal(b))
	if err != nil {
		return 0, err
	}
	return x.Cmp(y), nil
}

// AmountsEqual reports whether two amounts are numerically equal. A malformed amount is not equal
// to anything.
func AmountsEqual[A, B Amount](a A, b B) bool {
	cmp, err := CompareAmounts(a, b)
	return err == nil && cmp == 0
}

// AddAmounts adds two decimal amounts at the wider of their two scales.
func AddAmounts[A, B Amount](a A, b B) (Decimal, error) {
	x, y, err := alignedPair(Decimal(a), Decimal(b))
	if err != nil {
		return "", err
	}
	return Decimal(unscale(new(big.Int).Add(x, y), scaleOf(Decimal(a), Decimal(b)))), nil
}

// SubtractAmounts subtracts b from a at the wider of their two scales.
func SubtractAmounts[A, B Amount](a A, b B) (Decimal, error) {
	x, y, err := alignedPair(Decimal(a), Decimal(b))
	if err != nil {
		return "", err
	}
	return Decimal(unscale(new(big.Int).Sub(x, y), scaleOf(Decimal(a), Decimal(b)))), nil
}

// IsZeroAmount reports whether an amount is zero at any scale ("0", "0.000000").
func IsZeroAmount[A Amount](a A) bool {
	scaled, err := scale(string(a), fracLen(string(a)))
	return err == nil && scaled.Sign() == 0
}

func alignedPair(a, b Decimal) (*big.Int, *big.Int, error) {
	s := scaleOf(a, b)
	x, err := scale(string(a), s)
	if err != nil {
		return nil, nil, err
	}
	y, err := scale(string(b), s)
	if err != nil {
		return nil, nil, err
	}
	return x, y, nil
}

func scaleOf(amounts ...Decimal) int {
	widest := 0
	for _, a := range amounts {
		if n := fracLen(string(a)); n > widest {
			widest = n
		}
	}
	return widest
}

func fracLen(amount string) int {
	if dot := strings.IndexByte(amount, '.'); dot >= 0 {
		return len(amount) - dot - 1
	}
	return 0
}

// scale renders an amount as an integer of 10^-places units.
func scale(amount string, places int) (*big.Int, error) {
	if len(amount) > MaxAmountLength {
		return nil, badAmount(amount)
	}
	body := amount
	negative := strings.HasPrefix(body, "-")
	if negative {
		body = body[1:]
	}
	intPart, fracPart := body, ""
	hasDot := false
	if dot := strings.IndexByte(body, '.'); dot >= 0 {
		intPart, fracPart, hasDot = body[:dot], body[dot+1:], true
	}
	// A dot with nothing after it ("5.") is not an amount: reading it as "5" would quietly accept
	// a truncated string, and adding it to "1" would answer "6" as if nothing were wrong.
	if intPart == "" || (hasDot && fracPart == "") || !allDigits(intPart) || !allDigits(fracPart) {
		return nil, badAmount(amount)
	}
	digits := intPart + fracPart + strings.Repeat("0", places-len(fracPart))
	value, ok := new(big.Int).SetString(digits, 10)
	if !ok {
		return nil, badAmount(amount)
	}
	if negative {
		value.Neg(value)
	}
	return value, nil
}

// badAmount is the one error the money helpers report.
func badAmount(amount string) *Error {
	return newConfigError(CodeBadAmount, fmt.Sprintf(
		"%q is not a decimal amount: use digits with at most one dot, a non-empty integer part and a non-empty fraction (\"25\", \"10.000000\", \"-0.5\")",
		amount), "amount")
}

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// unscale renders an integer of 10^-places units back as a decimal string.
func unscale(value *big.Int, places int) string {
	sign := ""
	if value.Sign() < 0 {
		sign = "-"
		value = new(big.Int).Neg(value)
	}
	digits := value.String()
	if len(digits) <= places {
		digits = strings.Repeat("0", places-len(digits)+1) + digits
	}
	if places == 0 {
		return sign + digits
	}
	return sign + digits[:len(digits)-places] + "." + digits[len(digits)-places:]
}
