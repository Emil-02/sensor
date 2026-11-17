package slogsensor_test

import (
	"log/slog"
	"os"

	"github.com/Emil-02/sensor/slogsensor"
)

func ExampleNew() {
	base := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	})
	log := slog.New(slogsensor.New(base, slogsensor.Options{}))
	log.Info("pendaftaran", "nik", "3201014508900001", "nama", "Budi Santoso")
	// Output: level=INFO msg=pendaftaran nik=************0001 nama="B*** S******"
}

func ExampleNewWriter() {
	w, _ := slogsensor.NewWriter(os.Stdout, slogsensor.Options{})
	defer w.Close()
	w.Write([]byte("login dari 081234567890\n"))
	// Output: login dari ********7890
}
