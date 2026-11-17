// Package slogsensor redacts Indonesian personal data from log/slog records
// and from any line-based log stream.
//
//	logger := slog.New(slogsensor.New(slog.NewJSONHandler(os.Stdout, nil), slogsensor.Options{}))
//	logger.Info("daftar", "nik", "3201014508900001") // nik=************0001
package slogsensor

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"sync"

	"github.com/Emil-02/sensor/pii"
)

// Options configures redaction.
type Options struct {
	// Redact is passed to pii.Redact. The zero value masks values.
	Redact pii.Options
	// Types limits detection to these types (all types when empty).
	Types []pii.Type
}

// Handler wraps another slog.Handler and redacts the message and every
// string attribute before passing the record on.
type Handler struct {
	next    slog.Handler
	scanner *pii.Scanner
	opts    pii.Options
}

// New returns a redacting handler. It panics if opts.Redact is invalid
// (for example ModeHash without a key), since that is a programming error.
func New(next slog.Handler, opts Options) *Handler {
	h := &Handler{next: next, scanner: pii.New(opts.Types...), opts: opts.Redact}
	if _, _, err := h.scanner.Redact("", h.opts); err != nil {
		panic("slogsensor: " + err.Error())
	}
	return h
}

func (h *Handler) redact(s string) string {
	out, _, _ := h.scanner.Redact(s, h.opts)
	return out
}

// Enabled implements slog.Handler.
func (h *Handler) Enabled(ctx context.Context, l slog.Level) bool { return h.next.Enabled(ctx, l) }

// Handle implements slog.Handler.
func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	nr := slog.NewRecord(r.Time, r.Level, h.redact(r.Message), r.PC)
	r.Attrs(func(a slog.Attr) bool {
		nr.AddAttrs(h.attr(a))
		return true
	})
	return h.next.Handle(ctx, nr)
}

// WithAttrs implements slog.Handler.
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	red := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		red[i] = h.attr(a)
	}
	return &Handler{next: h.next.WithAttrs(red), scanner: h.scanner, opts: h.opts}
}

// WithGroup implements slog.Handler.
func (h *Handler) WithGroup(name string) slog.Handler {
	return &Handler{next: h.next.WithGroup(name), scanner: h.scanner, opts: h.opts}
}

func (h *Handler) attr(a slog.Attr) slog.Attr {
	v := a.Value.Resolve()
	switch v.Kind() {
	case slog.KindString:
		return slog.String(a.Key, h.redactKeyed(a.Key, v.String()))
	case slog.KindGroup:
		g := v.Group()
		out := make([]any, len(g))
		for i, x := range g {
			out[i] = h.attr(x)
		}
		return slog.Group(a.Key, out...)
	case slog.KindInt64, slog.KindUint64:
		s := v.String()
		if r := h.redactKeyed(a.Key, s); r != s {
			return slog.String(a.Key, r)
		}
	case slog.KindAny:
		if s, ok := v.Any().(interface{ String() string }); ok {
			str := s.String()
			if r := h.redactKeyed(a.Key, str); r != str {
				return slog.String(a.Key, r)
			}
		}
	}
	return slog.Attr{Key: a.Key, Value: v}
}

// redactKeyed treats the attribute key as context: values of keys such as
// "nama" or "alamat" are redacted entirely, and numbers are scanned with the
// key in front so slog.String("rekening", "1234567890") is recognised.
func (h *Handler) redactKeyed(key, val string) string {
	if t, ok := pii.FieldType(key); ok && h.scanner.Enabled(t) {
		out, _ := pii.RedactValue(t, val, h.opts)
		return out
	}
	prefix := key + ": "
	out, fs, _ := h.scanner.Redact(prefix+val, h.opts)
	if len(fs) == 0 {
		return val
	}
	if fs[0].Start < len(prefix) {
		return h.redact(val)
	}
	return out[len(prefix):]
}

// Writer redacts a line-based stream (for zap, zerolog, the standard log
// package or any io.Writer). Partial lines are buffered until a newline
// arrives or Flush/Close is called.
type Writer struct {
	mu      sync.Mutex
	w       io.Writer
	scanner *pii.Scanner
	opts    pii.Options
	buf     []byte
}

// maxLine bounds buffering: longer lines are redacted and written in chunks.
const maxLine = 1 << 20

// NewWriter returns a redacting writer.
func NewWriter(w io.Writer, opts Options) (*Writer, error) {
	s := pii.New(opts.Types...)
	if _, _, err := s.Redact("", opts.Redact); err != nil {
		return nil, err
	}
	return &Writer{w: w, scanner: s, opts: opts.Redact}, nil
}

// Write implements io.Writer.
func (w *Writer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			if len(w.buf) >= maxLine {
				if err := w.emit(w.buf); err != nil {
					return 0, err
				}
				w.buf = w.buf[:0]
			}
			return len(p), nil
		}
		if err := w.emit(w.buf[:i+1]); err != nil {
			return 0, err
		}
		w.buf = w.buf[i+1:]
	}
}

func (w *Writer) emit(line []byte) error {
	out, _, _ := w.scanner.Redact(string(line), w.opts)
	_, err := io.WriteString(w.w, out)
	return err
}

// Flush writes any buffered partial line.
func (w *Writer) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.buf) == 0 {
		return nil
	}
	err := w.emit(w.buf)
	w.buf = w.buf[:0]
	return err
}

// Close flushes the writer. It does not close the underlying writer.
func (w *Writer) Close() error { return w.Flush() }
