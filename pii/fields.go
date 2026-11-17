package pii

import (
	"regexp"
	"strings"
)

var emailRe = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9\-]+(?:\.[A-Za-z0-9\-]+)*\.[A-Za-z]{2,}`)

func scanEmails(text string) []Finding {
	var out []Finding
	for _, m := range emailRe.FindAllStringIndex(text, -1) {
		out = append(out, Finding{Type: Email, Start: m[0], End: m[1], Value: text[m[0]:m[1]]})
	}
	return out
}

// Labels printed on a KTP or used in forms, at the start of a line and
// followed by ':', ';' or '='. Group 1 is the label, group 2 the value.
// Labels tolerate common OCR misreadings ("Narna", "Tempau/Tgi Lahir",
// "RT1RW", "Kecamalan").
var lineFieldRe = regexp.MustCompile(`(?im)^[ \t]*(` + strings.Join([]string{
	`na(?:m|rn)a(?:[ \t]+lengkap)?`,
	`na(?:m|rn)a[ \t]+ibu(?:[ \t]+kandung)?`,
	`a[l1i]ama[tl](?:[ \t]+lengkap)?`,
	`temp[^\n:;=]{0,20}?lahir`,
	`t(?:g[l1i]|anggal)\.?[ \t]*lahir`,
	`rt[ \t]*[/\\|1il]?[ \t]*rw`,
	`ke[l1i][ \t]*[/\\|1il]?[ \t]*desa`,
	`kelurahan`,
	`desa`,
	`keca[a-z]{3,7}`,
}, "|") + `)[ \t]*(?:[-–—.,|_~]+[ \t]*)?[:;=][ \t]*([^\r\n]*?)[ \t]*$`)

// Structured keys in JSON ("key": "value") and key=value logs.
const fieldKeys = `nama|nama_lengkap|namalengkap|full_name|fullname|customer_name|nama_ibu|nama_ibu_kandung|mother_name|mothers_maiden_name|alamat|alamat_lengkap|address|home_address|tempat_lahir|birth_place|place_of_birth|tanggal_lahir|tgl_lahir|birth_date|birthdate|date_of_birth|dob`

var (
	jsonFieldRe = regexp.MustCompile(`(?i)"(` + fieldKeys + `)"\s*:\s*"((?:[^"\\\n]|\\.)*)"`)
	kvFieldRe   = regexp.MustCompile(`(?i)(?:^|[\s,;&?{(])(` + fieldKeys + `)=("[^"\n]*"|[^\s&,;)}]+)`)
)

var fieldKeySet = func() map[string]bool {
	m := map[string]bool{}
	for _, k := range strings.Split(fieldKeys, "|") {
		m[k] = true
	}
	return m
}()

// FieldType reports the type of a structured field key such as "nama",
// "full_name" or "tanggal_lahir" (case-insensitive).
func FieldType(key string) (Type, bool) {
	k := strings.ToLower(strings.TrimSpace(key))
	if !fieldKeySet[k] {
		return "", false
	}
	return fieldType(k), true
}

// "a.n. Siti Aminah" / "atas nama BUDI": up to four capitalised words.
var accountNameRe = regexp.MustCompile(`(?:^|[^A-Za-z])(?i:a\.n\.?|a/n|atas nama)[ \t]*:?[ \t]*([A-Z][A-Za-z'.-]*(?:[ \t]+[A-Z][A-Za-z'.-]*){0,3})`)

func fieldType(label string) Type {
	l := strings.ToLower(label)
	switch {
	case strings.Contains(l, "lahir"), strings.Contains(l, "birth"), l == "dob":
		return Birth
	case strings.HasPrefix(l, "nama"), strings.HasPrefix(l, "narna"), strings.Contains(l, "name"):
		return Name
	}
	return Address
}

func scanFields(text string) []Finding {
	var out []Finding
	add := func(label string, a, b int) {
		v := text[a:b]
		trimmed := strings.TrimSpace(v)
		if trimmed == "" || trimmed == "-" {
			return
		}
		a += strings.Index(v, trimmed)
		b = a + len(trimmed)
		out = append(out, Finding{Type: fieldType(label), Start: a, End: b, Value: text[a:b]})
	}
	for _, m := range lineFieldRe.FindAllStringSubmatchIndex(text, -1) {
		add(text[m[2]:m[3]], m[4], m[5])
	}
	for _, m := range jsonFieldRe.FindAllStringSubmatchIndex(text, -1) {
		add(text[m[2]:m[3]], m[4], m[5])
	}
	for _, m := range accountNameRe.FindAllStringSubmatchIndex(text, -1) {
		add("nama", m[2], m[3])
	}
	for _, m := range kvFieldRe.FindAllStringSubmatchIndex(text, -1) {
		a, b := m[4], m[5]
		if b-a >= 2 && text[a] == '"' && text[b-1] == '"' {
			a, b = a+1, b-1
		}
		add(text[m[2]:m[3]], a, b)
	}
	return out
}
