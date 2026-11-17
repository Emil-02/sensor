package pii

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"unicode"
	"unicode/utf8"
)

// keepDigits is how many trailing digits stay visible in ModeMask.
const keepDigits = 4

func mask(t Type, v string) string {
	switch t {
	case Email:
		return maskEmail(v)
	case Name:
		return maskWords(v)
	case Address, Birth:
		return maskAll(v)
	}
	return maskDigits(v)
}

// maskDigits hides every digit except the last keepDigits, keeping
// separators so the format stays recognisable: 0812-****-7890 style.
func maskDigits(v string) string {
	total := 0
	for i := 0; i < len(v); i++ {
		if isDigit(v[i]) {
			total++
		}
	}
	b := []byte(v)
	seen := 0
	for i := range b {
		if isDigit(b[i]) {
			seen++
			if seen <= total-keepDigits {
				b[i] = '*'
			}
		}
	}
	return string(b)
}

func maskEmail(v string) string {
	at := strings.LastIndexByte(v, '@')
	if at <= 0 {
		return maskAll(v)
	}
	r, size := utf8.DecodeRuneInString(v)
	if size >= at {
		return "*" + v[at:]
	}
	return string(r) + "***" + v[at:]
}

// maskWords keeps the first letter of each word: "BUDI SANTOSO" -> "B*** S******".
func maskWords(v string) string {
	var b strings.Builder
	first := true
	for _, r := range v {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if first {
				b.WriteRune(r)
			} else {
				b.WriteByte('*')
			}
			first = false
		default:
			b.WriteRune(r)
			first = true
		}
	}
	return b.String()
}

func maskAll(v string) string {
	var b strings.Builder
	for _, r := range v {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteByte('*')
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// canonical normalises a value so different spellings hash the same.
func canonical(f Finding) string {
	switch f.Type {
	case Email:
		return strings.ToLower(f.Value)
	case Name, Address, Birth:
		return strings.ToUpper(strings.Join(strings.Fields(f.Value), " "))
	case Phone:
		d := digitsOnly(f.Value)
		if strings.HasPrefix(d, "0") {
			d = "62" + d[1:]
		}
		return d
	}
	return digitsOnly(f.Value)
}

func hashValue(key []byte, f Finding) string {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(f.Type))
	m.Write([]byte{':'})
	m.Write([]byte(canonical(f)))
	return hex.EncodeToString(m.Sum(nil))[:10]
}
