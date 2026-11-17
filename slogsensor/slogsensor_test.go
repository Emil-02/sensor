package slogsensor

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/Emil-02/sensor/pii"
)

func newLogger(buf *bytes.Buffer, opts Options) *slog.Logger {
	return slog.New(New(slog.NewTextHandler(buf, &slog.HandlerOptions{
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	}), opts))
}

func TestHandler(t *testing.T) {
	var buf bytes.Buffer
	log := newLogger(&buf, Options{})
	log.With("user_phone", "081234567890").
		WithGroup("req").
		Info("daftar NIK 3201014508900001",
			"nama", "Budi Santoso",
			"rekening", "1234567890",
			"order", "INV-1234567890",
			"count", 3,
			slog.Group("ktp", "nik", "3201014508900001"),
		)
	got := buf.String()
	for _, leak := range []string{"081234567890", "3201014508900001", "Budi Santoso", "rekening=1234567890"} {
		if strings.Contains(got, leak) {
			t.Errorf("leaked %q in %s", leak, got)
		}
	}
	for _, want := range []string{
		`msg="daftar NIK ************0001"`, "user_phone=********7890", "req.nama=\"B*** S******\"",
		"req.rekening=******7890", "req.order=INV-1234567890", "req.count=3", "req.ktp.nik=************0001",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

func TestHandlerTypesAndModes(t *testing.T) {
	var buf bytes.Buffer
	log := newLogger(&buf, Options{Types: []pii.Type{pii.Phone}, Redact: pii.Options{Mode: pii.ModeLabel}})
	log.Info("x", "nama", "Budi", "hp", "081234567890")
	if got := buf.String(); !strings.Contains(got, "nama=Budi") || !strings.Contains(got, "hp=[PHONE]") {
		t.Fatalf("got %s", got)
	}

	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for hash mode without key")
		}
	}()
	New(slog.NewTextHandler(&buf, nil), Options{Redact: pii.Options{Mode: pii.ModeHash}})
}

func TestWriter(t *testing.T) {
	var buf bytes.Buffer
	w, err := NewWriter(&buf, Options{Redact: pii.Options{Mode: pii.ModeLabel}})
	if err != nil {
		t.Fatal(err)
	}
	// The number is split across writes; it must still be redacted.
	for _, p := range []string{"login hp 0812", "34567890 ok\nemail a@contoh.id", "\npartial 3201014508900001"} {
		if _, err := w.Write([]byte(p)); err != nil {
			t.Fatal(err)
		}
	}
	if strings.Contains(buf.String(), "partial") {
		t.Fatal("partial line should stay buffered")
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	want := "login hp [PHONE] ok\nemail [EMAIL]\npartial [NIK]"
	if buf.String() != want {
		t.Fatalf("got %q want %q", buf.String(), want)
	}
	if _, err := NewWriter(&buf, Options{Redact: pii.Options{Mode: pii.ModeHash}}); err == nil {
		t.Fatal("expected error")
	}
}
