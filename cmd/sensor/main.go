// Command sensor finds and redacts Indonesian personal data (NIK, KK, NPWP,
// phone numbers, bank accounts, cards, e-mail, KTP fields) in files and
// streams.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/Emil-02/sensor/pii"
)

var version = "dev"

const usage = `sensor — temukan & samarkan data pribadi Indonesia

Penggunaan:
  sensor scan   [flag] [file...]   laporkan temuan (exit 1 bila ada temuan)
  sensor redact [flag] [file...]   tulis versi tersamar ke stdout
  sensor types                     daftar jenis data
  sensor version

Tanpa file (atau "-"), input dibaca dari stdin:
  kubectl logs api | sensor redact
  sensor scan --json data.csv

Flag:
  --types nik,phone,...  jenis yang dideteksi (default: semua)
  --json                 (scan) keluaran JSON
  --mode mask|label|hash (redact) cara menyamarkan, default mask
  -w                     (redact) timpa file aslinya
Mode hash memakai kunci dari env SENSOR_HASH_KEY (minimal 16 byte).
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "scan":
		return cmdScan(args[1:], stdin, stdout, stderr)
	case "redact":
		return cmdRedact(args[1:], stdin, stdout, stderr)
	case "types":
		for _, t := range pii.AllTypes {
			fmt.Fprintf(stdout, "%-8s %s\n", t, pii.Labels[t])
		}
		return 0
	case "version", "--version", "-v":
		fmt.Fprintln(stdout, "sensor", version)
		return 0
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage)
		return 0
	}
	fmt.Fprintf(stderr, "perintah tidak dikenal: %s\n\n%s", args[0], usage)
	return 2
}

func parseTypes(s string) ([]pii.Type, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	var out []pii.Type
	for _, p := range strings.Split(s, ",") {
		t, ok := pii.ParseType(p)
		if !ok {
			return nil, fmt.Errorf("jenis tidak dikenal: %q (lihat: sensor types)", strings.TrimSpace(p))
		}
		out = append(out, t)
	}
	return out, nil
}

// eachLine calls fn for every line of r, including the trailing newline.
func eachLine(r io.Reader, fn func(line string) error) error {
	br := bufio.NewReaderSize(r, 64*1024)
	for {
		line, err := br.ReadString('\n')
		if line != "" {
			if ferr := fn(line); ferr != nil {
				return ferr
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func open(name string, stdin io.Reader) (io.Reader, func(), error) {
	if name == "-" {
		return stdin, func() {}, nil
	}
	f, err := os.Open(name)
	if err != nil {
		return nil, nil, err
	}
	return f, func() { f.Close() }, nil
}

type result struct {
	File   string   `json:"file"`
	Line   int      `json:"line"`
	Column int      `json:"column"`
	Type   pii.Type `json:"type"`
	Masked string   `json:"masked"`
}

func cmdScan(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("scan", flag.ContinueOnError)
	fs.SetOutput(stderr)
	typesFlag := fs.String("types", "", "jenis yang dideteksi")
	asJSON := fs.Bool("json", false, "keluaran JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	types, err := parseTypes(*typesFlag)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	scanner := pii.New(types...)
	files := fs.Args()
	if len(files) == 0 {
		files = []string{"-"}
	}

	results := []result{}
	failed := false
	for _, name := range files {
		r, closeFn, err := open(name, stdin)
		if err != nil {
			fmt.Fprintln(stderr, err)
			failed = true
			continue
		}
		display := name
		if name == "-" {
			display = "<stdin>"
		}
		n := 0
		err = eachLine(r, func(line string) error {
			n++
			for _, f := range scanner.Scan(line) {
				res := result{File: display, Line: n, Column: utf8.RuneCountInString(line[:f.Start]) + 1, Type: f.Type, Masked: f.Masked}
				results = append(results, res)
				if !*asJSON {
					fmt.Fprintf(stdout, "%s:%d:%d\t%s\t%s\n", res.File, res.Line, res.Column, pii.Labels[res.Type], res.Masked)
				}
			}
			return nil
		})
		closeFn()
		if err != nil {
			fmt.Fprintf(stderr, "%s: %v\n", name, err)
			failed = true
		}
	}
	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(results)
	} else if len(results) > 0 {
		fmt.Fprintf(stderr, "%d data pribadi ditemukan\n", len(results))
	}
	switch {
	case failed:
		return 2
	case len(results) > 0:
		return 1
	}
	return 0
}

func cmdRedact(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("redact", flag.ContinueOnError)
	fs.SetOutput(stderr)
	typesFlag := fs.String("types", "", "jenis yang dideteksi")
	mode := fs.String("mode", "mask", "mask, label atau hash")
	inPlace := fs.Bool("w", false, "timpa file aslinya")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	types, err := parseTypes(*typesFlag)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	opts := pii.Options{Mode: pii.Mode(*mode)}
	if opts.Mode == pii.ModeHash {
		opts.Key = []byte(os.Getenv("SENSOR_HASH_KEY"))
	}
	scanner := pii.New(types...)
	if _, _, err := scanner.Redact("", opts); err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	files := fs.Args()
	if len(files) == 0 {
		files = []string{"-"}
	}
	if *inPlace {
		for _, f := range files {
			if f == "-" {
				fmt.Fprintln(stderr, "-w tidak bisa dipakai dengan stdin")
				return 2
			}
		}
	}

	status := 0
	for _, name := range files {
		var err error
		if *inPlace {
			err = redactInPlace(name, scanner, opts)
		} else {
			err = redactTo(name, stdin, stdout, scanner, opts)
		}
		if err != nil {
			fmt.Fprintf(stderr, "%s: %v\n", name, err)
			status = 2
		}
	}
	return status
}

func redactTo(name string, stdin io.Reader, w io.Writer, s *pii.Scanner, opts pii.Options) error {
	r, closeFn, err := open(name, stdin)
	if err != nil {
		return err
	}
	defer closeFn()
	bw := bufio.NewWriter(w)
	err = eachLine(r, func(line string) error {
		out, _, _ := s.Redact(line, opts)
		_, werr := bw.WriteString(out)
		if name == "-" && strings.HasSuffix(line, "\n") {
			// Keep streaming output (e.g. tail -f | sensor redact) live.
			if ferr := bw.Flush(); ferr != nil {
				return ferr
			}
		}
		return werr
	})
	if ferr := bw.Flush(); err == nil {
		err = ferr
	}
	return err
}

func redactInPlace(name string, s *pii.Scanner, opts pii.Options) error {
	info, err := os.Stat(name)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(name), ".sensor-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := redactTo(name, nil, tmp, s, opts); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(info.Mode().Perm()); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), name)
}
