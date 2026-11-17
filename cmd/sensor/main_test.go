package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runCLI(t *testing.T, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := run(args, strings.NewReader(stdin), &out, &errb)
	return code, out.String(), errb.String()
}

const sample = "halo\nNIK: 3201014508900001\nWA 0812-3456-7890 dan email budi@contoh.id\n"

func TestScanText(t *testing.T) {
	code, out, errs := runCLI(t, sample, "scan")
	if code != 1 {
		t.Fatalf("code %d, stderr %s", code, errs)
	}
	for _, want := range []string{"<stdin>:2:6\tNIK\t************0001", "<stdin>:3:4\tNomor HP\t****-****-7890", "<stdin>:3:29\tEmail\tb***@contoh.id"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "3201014508900001") {
		t.Error("scan output must not contain raw values")
	}
}

func TestScanJSONAndTypes(t *testing.T) {
	code, out, _ := runCLI(t, sample, "scan", "--json", "--types", "phone")
	if code != 1 {
		t.Fatalf("code %d", code)
	}
	var res []result
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || res[0].Type != "phone" || res[0].Line != 3 {
		t.Fatalf("got %+v", res)
	}
	if code, out, _ := runCLI(t, "tidak ada apa-apa\n", "scan", "--json"); code != 0 || strings.TrimSpace(out) != "[]" {
		t.Fatalf("clean input: code %d out %q", code, out)
	}
	if code, _, _ := runCLI(t, "", "scan", "--types", "ktp"); code != 2 {
		t.Fatal("unknown type should fail")
	}
}

func TestRedact(t *testing.T) {
	code, out, _ := runCLI(t, sample, "redact", "--mode", "label")
	want := "halo\nNIK: [NIK]\nWA [PHONE] dan email [EMAIL]\n"
	if code != 0 || out != want {
		t.Fatalf("code %d\n got %q\nwant %q", code, out, want)
	}
	if code, _, errs := runCLI(t, sample, "redact", "--mode", "hash"); code != 2 || !strings.Contains(errs, "kunci") {
		t.Fatalf("hash without key: code %d %s", code, errs)
	}
	t.Setenv("SENSOR_HASH_KEY", "0123456789abcdef")
	if code, out, _ := runCLI(t, "hp 081234567890", "redact", "--mode", "hash"); code != 0 || !strings.HasPrefix(out, "hp [PHONE:") {
		t.Fatalf("hash: code %d %q", code, out)
	}
}

func TestRedactInPlace(t *testing.T) {
	p := filepath.Join(t.TempDir(), "data.txt")
	if err := os.WriteFile(p, []byte(sample), 0o640); err != nil {
		t.Fatal(err)
	}
	if code, _, errs := runCLI(t, "", "redact", "-w", p); code != 0 {
		t.Fatalf("code %d %s", code, errs)
	}
	b, _ := os.ReadFile(p)
	if strings.Contains(string(b), "3201014508900001") || !strings.Contains(string(b), "************0001") {
		t.Fatalf("not redacted: %s", b)
	}
	if info, _ := os.Stat(p); info.Mode().Perm() != 0o640 {
		t.Fatalf("mode changed to %v", info.Mode().Perm())
	}
	if code, _, _ := runCLI(t, "", "redact", "-w"); code != 2 {
		t.Fatal("-w with stdin should fail")
	}
	if code, _, _ := runCLI(t, "", "scan", filepath.Join(t.TempDir(), "missing")); code != 2 {
		t.Fatal("missing file should exit 2")
	}
}

func TestMisc(t *testing.T) {
	if code, out, _ := runCLI(t, "", "types"); code != 0 || !strings.Contains(out, "npwp") {
		t.Fatal("types")
	}
	if code, _, _ := runCLI(t, ""); code != 2 {
		t.Fatal("no args")
	}
	if code, _, _ := runCLI(t, "", "nope"); code != 2 {
		t.Fatal("unknown command")
	}
}
