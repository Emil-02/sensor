package pii

import (
	"strings"
)

// scanNumbers finds digit-based identifiers. A "run" is a sequence of digit
// groups joined by single separators (space, '-', '.'), optionally starting
// with '+'. Runs touching letters (hex hashes, UUIDs, product codes) are
// ignored. Space-separated segments are tried longest-first so that
// "NIK 3201 0123 4567 8901" and "0812 3456 7890" are found as one value.
func scanNumbers(text string) []Finding {
	var out []Finding
	prevEnd := 0
	for i := 0; i < len(text); {
		c := text[i]
		startsRun := isDigit(c) || (c == '+' && i+1 < len(text) && isDigit(text[i+1]))
		if !startsRun || (i > 0 && (isAlnum(text[i-1]) || text[i-1] == '+')) {
			i++
			continue
		}
		end := runEnd(text, i)
		if end < len(text) && isLetter(text[end]) {
			prevEnd = end
			i = skipWord(text, end)
			continue
		}
		out = append(out, classifyRun(text, i, end, prevEnd)...)
		prevEnd = end
		i = end
	}
	return out
}

func runEnd(text string, i int) int {
	j := i
	if text[j] == '+' {
		j++
	}
	for {
		for j < len(text) && isDigit(text[j]) {
			j++
		}
		if j+1 < len(text) && (text[j] == ' ' || text[j] == '-' || text[j] == '.') && isDigit(text[j+1]) {
			j++
			continue
		}
		return j
	}
}

func skipWord(text string, i int) int {
	for i < len(text) && (isAlnum(text[i]) || text[i] == '_') {
		i++
	}
	return i
}

type segment struct{ start, end int }

// maxSegments bounds how many space-separated groups may form one value.
const maxSegments = 6

func classifyRun(text string, start, end, prevEnd int) []Finding {
	var segs []segment
	s := start
	for k := start; k < end; k++ {
		if text[k] == ' ' {
			segs = append(segs, segment{s, k})
			s = k + 1
		}
	}
	segs = append(segs, segment{s, end})

	var out []Finding
	ctxFloor := prevEnd
	for i := 0; i < len(segs); {
		matched := false
		for j := min(len(segs), i+maxSegments) - 1; j >= i; j-- {
			a, b := segs[i].start, segs[j].end
			ctx := context(text, a, ctxFloor)
			if t, ok := classify(text[a:b], ctx); ok {
				out = append(out, Finding{Type: t, Start: a, End: b, Value: text[a:b]})
				ctxFloor = b
				i = j + 1
				matched = true
				break
			}
		}
		if !matched {
			ctxFloor = segs[i].end
			i++
		}
	}
	return out
}

// contextWindow is how many bytes before a number are searched for keywords.
const contextWindow = 40

// context returns the lowercase text before position a on the same line,
// limited to contextWindow bytes and to text after the previous number.
func context(text string, a, floor int) string {
	lo := max(a-contextWindow, floor, 0)
	if nl := strings.LastIndexByte(text[lo:a], '\n'); nl >= 0 {
		lo += nl + 1
	}
	return strings.ToLower(text[lo:a])
}

var (
	kwNIK     = []string{"nik", "ktp", "nomor induk", "no induk", "e-ktp", "id_number", "national_id"}
	kwKK      = []string{"kk", "no_kk", "nokk", "kartu keluarga", "family card"}
	kwNPWP    = []string{"npwp", "tax id", "tax_id"}
	kwBPJS    = []string{"bpjs", "jkn", "kis", "no_bpjs"}
	kwCard    = []string{"kartu kredit", "credit card", "card", "cc", "kartu debit", "debit card"}
	kwAccount = []string{
		"rek", "rekening", "norek", "no.rek", "no rek", "no_rek", "a/n", "a.n", "atas nama", "account",
		"acc", "acct", "va", "virtual account", "transfer ke", "tf ke", "bank",
		"bca", "bri", "bni", "mandiri", "bsi", "cimb", "permata", "danamon", "btn", "btpn", "jenius",
		"jago", "seabank", "blu", "ocbc", "panin", "maybank", "mega", "muamalat", "bjb", "allo",
	}
)

func hasKeyword(ctx string, kws []string) bool {
	for _, kw := range kws {
		for off := 0; ; {
			k := strings.Index(ctx[off:], kw)
			if k < 0 {
				break
			}
			a, b := off+k, off+k+len(kw)
			if (a == 0 || !isLetter(ctx[a-1])) && (b == len(ctx) || !isLetter(ctx[b])) {
				return true
			}
			off = a + 1
		}
	}
	return false
}

func classify(raw, ctx string) (Type, bool) {
	d := digitsOnly(raw)
	n := len(d)
	plus := strings.HasPrefix(raw, "+")
	switch {
	case hasKeyword(ctx, kwNPWP) && (n == 15 || n == 16):
		return NPWP, true
	case hasKeyword(ctx, kwKK) && n == 16:
		return KK, true
	case hasKeyword(ctx, kwBPJS) && n == 13:
		return BPJS, true
	case n == 15 && isNPWPFormatted(raw):
		return NPWP, true
	case n == 16 && !plus && (hasKeyword(ctx, kwNIK) || ValidNIK(d)):
		return NIK, true
	case isPhone(d, plus):
		return Phone, true
	case !plus && isCard(d) && (hasKeyword(ctx, kwCard) || separatorsOK(raw)):
		return Card, true
	case !plus && n >= 7 && n <= 18 && hasKeyword(ctx, kwAccount):
		return Account, true
	}
	return "", false
}

// separatorsOK accepts card numbers written contiguously or in groups
// separated by spaces/dashes, but not dotted numbers (amounts).
func separatorsOK(raw string) bool { return !strings.Contains(raw, ".") }

func isNPWPFormatted(raw string) bool {
	// 99.999.999.9-999.999
	const pat = "dd.ddd.ddd.d-ddd.ddd"
	if len(raw) != len(pat) {
		return false
	}
	for i := 0; i < len(pat); i++ {
		if pat[i] == 'd' {
			if !isDigit(raw[i]) {
				return false
			}
		} else if raw[i] != pat[i] {
			return false
		}
	}
	return true
}

func isPhone(d string, plus bool) bool {
	var local string
	switch {
	case strings.HasPrefix(d, "62"):
		local = "0" + d[2:]
	case plus:
		return false
	default:
		local = d
	}
	return len(local) >= 10 && len(local) <= 13 && local[0] == '0' && local[1] == '8' && local[2] != '0'
}

func isCard(d string) bool {
	n := len(d)
	if n < 13 || n > 19 || !luhn(d) {
		return false
	}
	p2 := atoi(d[:2])
	p4 := atoi(d[:4])
	switch {
	case d[0] == '4':
		return n == 13 || n == 16 || n == 19
	case p2 >= 51 && p2 <= 55, p4 >= 2221 && p4 <= 2720:
		return n == 16
	case p2 == 34 || p2 == 37:
		return n == 15
	case p2 == 35, p2 == 62, p2 == 65, p4 == 6011:
		return n >= 16
	case p2 == 36 || p2 == 38 || p2 == 30:
		return n == 14
	}
	return false
}

func luhn(d string) bool {
	sum := 0
	alt := false
	for i := len(d) - 1; i >= 0; i-- {
		v := int(d[i] - '0')
		if alt {
			v *= 2
			if v > 9 {
				v -= 9
			}
		}
		sum += v
		alt = !alt
	}
	return sum%10 == 0
}

var provinces = map[int]bool{
	11: true, 12: true, 13: true, 14: true, 15: true, 16: true, 17: true, 18: true, 19: true,
	21: true, 31: true, 32: true, 33: true, 34: true, 35: true, 36: true,
	51: true, 52: true, 53: true, 61: true, 62: true, 63: true, 64: true, 65: true,
	71: true, 72: true, 73: true, 74: true, 75: true, 76: true, 81: true, 82: true,
	91: true, 92: true, 93: true, 94: true, 95: true, 96: true,
}

// ValidNIK reports whether d (16 digits) has the structure of an Indonesian
// NIK: province, regency and district codes, date of birth (day +40 for
// women) and a non-zero serial number. It cannot tell whether the NIK exists.
func ValidNIK(d string) bool {
	if len(d) != 16 || digitsOnly(d) != d {
		return false
	}
	if !provinces[atoi(d[0:2])] || atoi(d[2:4]) == 0 || atoi(d[4:6]) == 0 {
		return false
	}
	day, month := atoi(d[6:8]), atoi(d[8:10])
	if day > 40 {
		day -= 40
	}
	return day >= 1 && day <= 31 && month >= 1 && month <= 12 && atoi(d[12:16]) != 0
}

func digitsOnly(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if isDigit(s[i]) {
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

func atoi(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		n = n*10 + int(s[i]-'0')
	}
	return n
}

func isDigit(c byte) bool  { return c >= '0' && c <= '9' }
func isLetter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }
func isAlnum(c byte) bool  { return isDigit(c) || isLetter(c) }
