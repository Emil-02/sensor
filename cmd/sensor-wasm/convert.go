// Command sensor-wasm exposes the pii package to JavaScript. Build with:
//
//	GOOS=js GOARCH=wasm go build -o web/sensor.wasm ./cmd/sensor-wasm
//
// It registers a global object `sensor` with two functions that return JSON:
//
//	sensor.scan(text, types)         -> [{type, start, end, masked, label}]
//	sensor.redact(text, mode, types) -> {text, findings}
//
// start/end are UTF-16 offsets, so they can be used with String.slice.
package main

import (
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/Emil-02/sensor/pii"
)

type jsFinding struct {
	Type   pii.Type `json:"type"`
	Label  string   `json:"label"`
	Start  int      `json:"start"`
	End    int      `json:"end"`
	Masked string   `json:"masked"`
}

func scannerFor(types string) *pii.Scanner {
	var ts []pii.Type
	for _, p := range strings.Split(types, ",") {
		if t, ok := pii.ParseType(p); ok {
			ts = append(ts, t)
		}
	}
	return pii.New(ts...)
}

// toUTF16 converts byte offsets of findings in text to UTF-16 offsets.
func toUTF16(text string, fs []pii.Finding) []jsFinding {
	out := make([]jsFinding, 0, len(fs))
	byteOff, u16 := 0, 0
	advance := func(to int) {
		for byteOff < to {
			r, size := utf8.DecodeRuneInString(text[byteOff:])
			if r >= 0x10000 {
				u16 += 2
			} else {
				u16++
			}
			byteOff += size
		}
	}
	for _, f := range fs {
		advance(f.Start)
		start := u16
		advance(f.End)
		out = append(out, jsFinding{Type: f.Type, Label: pii.Labels[f.Type], Start: start, End: u16, Masked: f.Masked})
	}
	return out
}

func scanJSON(text, types string) string {
	b, _ := json.Marshal(toUTF16(text, scannerFor(types).Scan(text)))
	return string(b)
}

func redactJSON(text, mode, types string) string {
	out, fs, err := scannerFor(types).Redact(text, pii.Options{Mode: pii.Mode(mode)})
	if err != nil {
		b, _ := json.Marshal(map[string]string{"error": err.Error()})
		return string(b)
	}
	b, _ := json.Marshal(map[string]any{"text": out, "findings": toUTF16(text, fs)})
	return string(b)
}
