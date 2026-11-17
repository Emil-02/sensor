package main

import (
	"encoding/json"
	"testing"
	"unicode/utf16"
)

func TestUTF16Offsets(t *testing.T) {
	text := "😀 é NIK 3201014508900001 😀 HP 081234567890"
	var fs []jsFinding
	if err := json.Unmarshal([]byte(scanJSON(text, "")), &fs); err != nil {
		t.Fatal(err)
	}
	u := utf16.Encode([]rune(text))
	want := []string{"3201014508900001", "081234567890"}
	if len(fs) != len(want) {
		t.Fatalf("got %+v", fs)
	}
	for i, f := range fs {
		if got := string(utf16.Decode(u[f.Start:f.End])); got != want[i] {
			t.Errorf("finding %d = %q, want %q", i, got, want[i])
		}
	}
}

func TestRedactJSON(t *testing.T) {
	var res struct {
		Text     string      `json:"text"`
		Findings []jsFinding `json:"findings"`
		Error    string      `json:"error"`
	}
	if err := json.Unmarshal([]byte(redactJSON("HP 081234567890 NIK 3201014508900001", "label", "phone")), &res); err != nil {
		t.Fatal(err)
	}
	if res.Text != "HP [PHONE] NIK 3201014508900001" || len(res.Findings) != 1 || res.Findings[0].Label != "Nomor HP" {
		t.Fatalf("got %+v", res)
	}
	_ = json.Unmarshal([]byte(redactJSON("x", "hash", "")), &res)
	if res.Error == "" {
		t.Fatal("hash mode without key must fail")
	}
}
