package pii

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// All identifiers below are synthetic test values.

func types(fs []Finding) []string {
	var out []string
	for _, f := range fs {
		out = append(out, string(f.Type)+"="+f.Value)
	}
	return out
}

func TestScan(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"nik plain", "NIK saya 3201014508900001 ya", []string{"nik=3201014508900001"}},
		{"nik no context", "data: 3171010101850002", []string{"nik=3171010101850002"}},
		{"nik spaced ocr", "NIK : 3201 0145 0890 0001", []string{"nik=3201 0145 0890 0001"}},
		{"nik invalid without context", "order 9901014508900001", nil},
		{"nik invalid with context", "nik=9901014508900001", []string{"nik=9901014508900001"}},
		{"kk context", "No. KK: 3201011203150004", []string{"kk=3201011203150004"}},
		{"npwp formatted", "NPWP 12.345.678.9-012.345", []string{"npwp=12.345.678.9-012.345"}},
		{"npwp 16 digit", "npwp: 3201014508900001", []string{"npwp=3201014508900001"}},
		{"phone 08", "hubungi 081234567890", []string{"phone=081234567890"}},
		{"phone +62 spaced", "WA +62 812 3456 7890.", []string{"phone=+62 812 3456 7890"}},
		{"phone dashed", "telp 0812-3456-7890", []string{"phone=0812-3456-7890"}},
		{"phone 62", `{"phone":"6281234567890"}`, []string{"phone=6281234567890"}},
		{"account with bank", "transfer ke BCA 1234567890 a.n. toko", []string{"account=1234567890"}},
		{"account holder name", "rek BRI 0123456789 a.n. Siti Aminah, makasih", []string{"account=0123456789", "name=Siti Aminah"}},
		{"atas nama upper", "atas nama: BUDI SANTOSO", []string{"name=BUDI SANTOSO"}},
		{"an lowercase word ignored", "kirim an. barang", nil},
		{"account rek", "no rek: 0123-456-789", []string{"account=0123-456-789"}},
		{"account without context", "invoice 1234567890", nil},
		{"card", "kartu 4111 1111 1111 1111 exp", []string{"card=4111 1111 1111 1111"}},
		{"card bad luhn", "kartu 4111 1111 1111 1112", nil},
		{"bpjs", "BPJS: 0001234567890", []string{"bpjs=0001234567890"}},
		{"email", "kirim ke budi.santoso+tes@contoh.co.id ya", []string{"email=budi.santoso+tes@contoh.co.id"}},
		{
			"ktp ocr",
			"PROVINSI JAWA BARAT\nNIK : 3201014508900001\nNama : SITI AMINAH\nTempat/Tgl Lahir : BOGOR, 05-08-1990\nAlamat : JL. MAWAR NO. 12\nRT/RW : 002/005\nKel/Desa : CIBINONG\nKecamatan : CIBINONG\nAgama : ISLAM",
			[]string{"nik=3201014508900001", "name=SITI AMINAH", "birth=BOGOR, 05-08-1990", "address=JL. MAWAR NO. 12", "address=002/005", "address=CIBINONG", "address=CIBINONG"},
		},
		{"json fields", `{"full_name":"Budi Santoso","alamat":"Jl. Melati 3","dob":"1990-01-02"}`, []string{"name=Budi Santoso", "address=Jl. Melati 3", "birth=1990-01-02"}},
		{"kv fields", `level=info nama="Budi Santoso" alamat=Bandung`, []string{"name=Budi Santoso", "address=Bandung"}},
		{"empty field", "Nama : -", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := types(Scan(c.in))
			if strings.Join(got, "|") != strings.Join(c.want, "|") {
				t.Fatalf("Scan(%q)\n got  %v\n want %v", c.in, got, c.want)
			}
		})
	}
}

// Numbers that commonly appear in logs and chats must not be reported.
func TestNoFalsePositives(t *testing.T) {
	inputs := []string{
		"2026-11-17T10:30:00Z request took 1234567890ns",
		"order_id=INV20261117000123 total=Rp 1.500.000",
		"Total belanja Rp 12.345.678.900",
		"uuid 550e8400-e29b-41d4-a716-446655440000",
		"trace=12345678-1234-1234-1234-123456789012",
		"sha256 3a7bd3e2360a3d29eea436fcfb7e44c735d117c42d1c1835420b6b9942dd4f1c",
		"timestamp 1763350000123",
		"resi JNE 0123456789012345 dikirim",
		"versi 1.2.3.4 build 20261117",
		"kode pos 16911, RT 002 RW 005",
		"harga 150000 qty 3",
		"tracking id 9876543210123456",
		"ip 192.168.100.200 port 8080",
		"jam 08.30 - 12.00",
	}
	for _, in := range inputs {
		if fs := Scan(in); len(fs) > 0 {
			t.Errorf("Scan(%q) = %v, want none", in, types(fs))
		}
	}
}

func TestValidNIK(t *testing.T) {
	for _, d := range []string{"3201014508900001", "3171010101850002", "9401017112990001"} {
		if !ValidNIK(d) {
			t.Errorf("ValidNIK(%s) = false", d)
		}
	}
	for _, d := range []string{"0001014508900001", "3200014508900001", "3201013208900001", "3201010113900001", "3201014508900000", "320101450890000", "320101450890000a"} {
		if ValidNIK(d) {
			t.Errorf("ValidNIK(%s) = true", d)
		}
	}
}

func TestRedactModes(t *testing.T) {
	in := "NIK 3201014508900001, HP 0812-3456-7890, email budi@contoh.id\nNama : BUDI SANTOSO"
	out, fs, err := Redact(in, Options{})
	if err != nil {
		t.Fatal(err)
	}
	want := "NIK ************0001, HP ****-****-7890, email b***@contoh.id\nNama : B*** S******"
	if out != want || len(fs) != 4 {
		t.Fatalf("mask:\n got  %q\n want %q", out, want)
	}

	out, _, _ = Redact(in, Options{Mode: ModeLabel})
	want = "NIK [NIK], HP [PHONE], email [EMAIL]\nNama : [NAME]"
	if out != want {
		t.Fatalf("label:\n got  %q\n want %q", out, want)
	}

	key := []byte("0123456789abcdef0123456789abcdef")
	a, _, _ := Redact("HP 0812-3456-7890", Options{Mode: ModeHash, Key: key})
	b, _, _ := Redact("HP +6281234567890", Options{Mode: ModeHash, Key: key})
	if a != b || !strings.HasPrefix(a, "HP [PHONE:") {
		t.Fatalf("hash should be stable across formats: %q vs %q", a, b)
	}
	if _, _, err := Redact(in, Options{Mode: ModeHash}); err != ErrKey {
		t.Fatalf("want ErrKey, got %v", err)
	}
	if _, _, err := Redact(in, Options{Mode: "x"}); err == nil {
		t.Fatal("want error for unknown mode")
	}
}

func TestScannerTypes(t *testing.T) {
	s := New(Phone)
	fs := s.Scan("NIK 3201014508900001 HP 081234567890")
	if len(fs) != 1 || fs[0].Type != Phone {
		t.Fatalf("got %v", types(fs))
	}
	if _, ok := ParseType(" NIK "); !ok {
		t.Fatal("ParseType should be case-insensitive")
	}
}

func TestFindingOffsets(t *testing.T) {
	in := "Halo 👋 nomor saya 081234567890"
	fs := Scan(in)
	if len(fs) != 1 || in[fs[0].Start:fs[0].End] != fs[0].Value {
		t.Fatalf("bad offsets: %+v", fs)
	}
}

func FuzzRedact(f *testing.F) {
	for _, s := range []string{
		"NIK 3201014508900001", "+62 812 3456 7890", "rek BCA 1234567890", "Nama : BUDI\nAlamat : JL X",
		`{"nama":"a\"b"}`, "4111-1111-1111-1111", "a@b.co", "npwp 12.345.678.9-012.345",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		out, fs, err := Redact(in, Options{Mode: ModeLabel})
		if err != nil {
			t.Fatal(err)
		}
		prev := 0
		for _, x := range fs {
			if x.Start < prev || x.End <= x.Start || in[x.Start:x.End] != x.Value {
				t.Fatalf("bad finding %+v in %q", x, in)
			}
			prev = x.End
			v := x.Value
			if len(v) >= 6 && strings.ContainsAny(v, "0123456789@") && strings.Count(in, v) == 1 && strings.Contains(out, v) {
				t.Fatalf("value %q leaked: %q", v, out)
			}
		}
		if utf8.ValidString(in) && !utf8.ValidString(out) {
			t.Fatalf("invalid UTF-8 output for %q", in)
		}
		if _, _, err := Redact(in, Options{}); err != nil {
			t.Fatal(err)
		}
	})
}
