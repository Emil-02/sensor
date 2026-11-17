// Package pii finds and redacts Indonesian personal data in free text:
// NIK, No. KK, NPWP, phone numbers, bank accounts, payment cards, BPJS
// numbers, e-mail addresses and labelled identity fields (name, address,
// place/date of birth) such as those printed on a KTP.
//
// Detection is rule based and runs offline. Numbers that are ambiguous on
// their own (bank accounts, BPJS, No. KK) are only reported when a keyword
// such as "rek", "BPJS" or "KK" appears shortly before them on the same line.
package pii

import (
	"errors"
	"sort"
	"strings"
)

// Type is the kind of personal data.
type Type string

// Supported types.
const (
	NIK     Type = "nik"
	KK      Type = "kk"
	NPWP    Type = "npwp"
	Phone   Type = "phone"
	Account Type = "account"
	Card    Type = "card"
	BPJS    Type = "bpjs"
	Email   Type = "email"
	Name    Type = "name"
	Address Type = "address"
	Birth   Type = "birth"
)

// AllTypes lists every supported type in a stable order.
var AllTypes = []Type{NIK, KK, NPWP, Phone, Account, Card, BPJS, Email, Name, Address, Birth}

// Labels are human readable names in Indonesian.
var Labels = map[Type]string{
	NIK:     "NIK",
	KK:      "Nomor KK",
	NPWP:    "NPWP",
	Phone:   "Nomor HP",
	Account: "Nomor rekening",
	Card:    "Kartu kredit/debit",
	BPJS:    "Nomor BPJS",
	Email:   "Email",
	Name:    "Nama",
	Address: "Alamat",
	Birth:   "Tempat/tanggal lahir",
}

// ParseType returns the Type for s (case-insensitive).
func ParseType(s string) (Type, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	for _, t := range AllTypes {
		if string(t) == s {
			return t, true
		}
	}
	return "", false
}

// Finding is a piece of personal data found in a text.
type Finding struct {
	Type Type `json:"type"`
	// Start and End are byte offsets: text[Start:End] == Value.
	Start int `json:"start"`
	End   int `json:"end"`
	// Value is the original text. It is never serialised.
	Value string `json:"-"`
	// Masked is a safe preview of Value.
	Masked string `json:"masked"`
}

// Scanner detects personal data. The zero value is not usable; use New.
type Scanner struct {
	enabled map[Type]bool
}

// New returns a Scanner for the given types (all types when none given).
func New(types ...Type) *Scanner {
	if len(types) == 0 {
		types = AllTypes
	}
	s := &Scanner{enabled: map[Type]bool{}}
	for _, t := range types {
		s.enabled[t] = true
	}
	return s
}

// Enabled reports whether the scanner detects type t.
func (s *Scanner) Enabled(t Type) bool { return s.enabled[t] }

var defaultScanner = New()

// Scan finds personal data of every type in text.
func Scan(text string) []Finding { return defaultScanner.Scan(text) }

// Scan finds personal data in text. Findings are sorted and never overlap.
func (s *Scanner) Scan(text string) []Finding {
	var all []Finding
	all = append(all, scanFields(text)...)
	all = append(all, scanEmails(text)...)
	all = append(all, scanNumbers(text)...)

	kept := all[:0]
	for _, f := range all {
		if s.enabled[f.Type] {
			kept = append(kept, f)
		}
	}
	sort.SliceStable(kept, func(i, j int) bool {
		if kept[i].Start != kept[j].Start {
			return kept[i].Start < kept[j].Start
		}
		return kept[i].End > kept[j].End
	})
	out := []Finding{}
	end := -1
	for _, f := range kept {
		if f.Start < end {
			continue
		}
		f.Masked = mask(f.Type, f.Value)
		out = append(out, f)
		end = f.End
	}
	return out
}

// Mode selects how findings are replaced.
type Mode string

// Redaction modes.
const (
	// ModeMask keeps a few characters (e.g. the last four digits).
	ModeMask Mode = "mask"
	// ModeLabel replaces the value with its type, e.g. "[NIK]".
	ModeLabel Mode = "label"
	// ModeHash replaces the value with a keyed hash, e.g. "[NIK:3f9a1c07b2]",
	// so the same value can still be correlated across log lines.
	ModeHash Mode = "hash"
)

// Options configures Redact.
type Options struct {
	Mode Mode
	// Key is the HMAC key for ModeHash (at least 16 bytes).
	Key []byte
}

// ErrKey is returned when ModeHash is used without a long enough key.
var ErrKey = errors.New("mode hash membutuhkan kunci minimal 16 byte")

func (o Options) normalize() (Options, error) {
	switch o.Mode {
	case "":
		o.Mode = ModeMask
	case ModeMask, ModeLabel:
	case ModeHash:
		if len(o.Key) < 16 {
			return o, ErrKey
		}
	default:
		return o, errors.New("mode tidak dikenal: " + string(o.Mode))
	}
	return o, nil
}

// Redact replaces every finding in text.
func Redact(text string, opts Options) (string, []Finding, error) {
	return defaultScanner.Redact(text, opts)
}

// Redact replaces every finding in text according to opts.
func (s *Scanner) Redact(text string, opts Options) (string, []Finding, error) {
	opts, err := opts.normalize()
	if err != nil {
		return "", nil, err
	}
	fs := s.Scan(text)
	if len(fs) == 0 {
		return text, fs, nil
	}
	var b strings.Builder
	b.Grow(len(text))
	last := 0
	for _, f := range fs {
		b.WriteString(text[last:f.Start])
		b.WriteString(replacement(f, opts))
		last = f.End
	}
	b.WriteString(text[last:])
	return b.String(), fs, nil
}

// RedactValue replaces a whole value already known to be of type t, for
// example the value of a "nama" field. Empty values are returned unchanged.
func RedactValue(t Type, v string, opts Options) (string, error) {
	opts, err := opts.normalize()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(v) == "" {
		return v, nil
	}
	f := Finding{Type: t, End: len(v), Value: v, Masked: mask(t, v)}
	return replacement(f, opts), nil
}

func replacement(f Finding, opts Options) string {
	switch opts.Mode {
	case ModeLabel:
		return "[" + strings.ToUpper(string(f.Type)) + "]"
	case ModeHash:
		return "[" + strings.ToUpper(string(f.Type)) + ":" + hashValue(opts.Key, f) + "]"
	}
	return f.Masked
}
