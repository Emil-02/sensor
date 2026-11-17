# sensor

[![CI](https://github.com/Emil-02/sensor/actions/workflows/ci.yml/badge.svg)](https://github.com/Emil-02/sensor/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/Emil-02/sensor.svg)](https://pkg.go.dev/github.com/Emil-02/sensor)
[![License: MIT](https://img.shields.io/badge/license-MIT-green)](LICENSE)
[![spruce](https://gh-spruce.vercel.app/api/badge?repo=Emil-02/sensor)](https://gh-spruce.vercel.app/u/Emil-02)

**Samarkan data pribadi Indonesia sebelum dikirim atau ditulis ke log.** NIK, nomor KK, NPWP, nomor HP, rekening, kartu, BPJS, email, serta nama, alamat, dan tanggal lahir di KTP.

**🌐 Coba langsung di browser: [emil-02.github.io/sensor](https://emil-02.github.io/sensor/)**. Foto dan teks diproses 100% di perangkat Anda dan tidak pernah di-upload.

*English: sensor finds and redacts Indonesian personal data (NIK, KK, NPWP, phone numbers, bank accounts, KTP fields) in photos, text and logs. It ships as a Go library, a `log/slog` handler, a CLI and an offline web app.*

## Kenapa?

- **Masyarakat:** foto KTP yang dikirim ke "penjual", "HRD", atau "verifikasi" pinjol sering disalahgunakan untuk pinjaman atas nama orang lain. Dengan sensor, Anda bisa menutup data yang tidak diperlukan dan menambahkan watermark *"Hanya untuk verifikasi X · tanggal"*, sehingga foto itu tidak bisa dipakai ulang di tempat lain.
- **Developer:** log aplikasi sering berisi NIK, nomor HP, dan rekening pelanggan. Menurut UU PDP No. 27/2022, itu kebocoran data. sensor membersihkan log di sumbernya (`slog`) atau di pipeline (`kubectl logs | sensor redact`).

## Web app (tanpa instalasi)

Buka **https://emil-02.github.io/sensor/**:

- **Foto KTP/dokumen:** OCR bahasa Indonesia menandai NIK, nama, alamat, dan data lain lalu menutupnya dengan **warna solid** (bukan blur, karena blur bisa dipulihkan). Anda bisa menambah kotak manual (misalnya untuk foto wajah atau tanda tangan) dan memasang watermark tujuan, lalu mengunduh PNG yang **bebas metadata EXIF/GPS**.
- **Teks:** tempel chat atau data, lalu salin versi yang sudah tersamar.
- Semua kode dan data OCR disajikan dari situs ini sendiri. CSP yang ketat (`connect-src 'self'`) mencegah halaman mengirim data ke server lain.

> OCR bisa salah baca pada foto miring atau buram. Selalu periksa pratinjau sebelum mengunduh.

## CLI

```bash
go install github.com/Emil-02/sensor/cmd/sensor@latest
```

```bash
sensor scan data.csv                 # laporkan temuan; exit 1 bila ada (cocok untuk CI)
sensor scan --json --types nik,phone dump.sql
sensor redact chat.txt > aman.txt    # samarkan (default: sisakan 4 digit terakhir)
sensor redact --mode label app.log   # ganti dengan [NIK], [PHONE], ...
sensor redact -w export/*.csv        # timpa file aslinya
kubectl logs -f api | sensor redact  # streaming
SENSOR_HASH_KEY=... sensor redact --mode hash app.log  # [PHONE:3f9a1c07b2], tetap bisa dikorelasikan
sensor types                         # daftar jenis data
```

Contoh keluaran `sensor scan`:

```text
data.csv:2:6     NIK        ************0001
data.csv:3:4     Nomor HP   ****-****-7890
```

Keluaran `scan` tidak pernah menampilkan nilai asli, jadi aman dipakai di log CI.

## Library Go

```go
import "github.com/Emil-02/sensor/pii"

out, findings, err := pii.Redact("NIK 3201014508900001, WA 0812-3456-7890", pii.Options{})
// out == "NIK ************0001, WA ****-****-7890"

s := pii.New(pii.Phone, pii.Email) // hanya jenis tertentu
out, _, _ = s.Redact(text, pii.Options{Mode: pii.ModeLabel})
```

### log/slog

```go
import "github.com/Emil-02/sensor/slogsensor"

logger := slog.New(slogsensor.New(slog.NewJSONHandler(os.Stdout, nil), slogsensor.Options{}))
logger.Info("pendaftaran", "nik", "3201014508900001", "nama", "Budi Santoso")
// {"msg":"pendaftaran","nik":"************0001","nama":"B*** S******"}
```

Nama key ikut dipakai sebagai konteks: `"rekening", "1234567890"` dikenali sebagai rekening, dan `"nama"`/`"alamat"`/`"tanggal_lahir"` selalu disamarkan. Untuk zap, zerolog, atau `log` standar, bungkus output-nya dengan `slogsensor.NewWriter(os.Stderr, slogsensor.Options{})`.

## Yang dikenali

| Jenis | Aturan |
|---|---|
| NIK | 16 digit dengan struktur valid (kode provinsi, kab/kota, kecamatan, tanggal lahir, perempuan +40, nomor urut). Selalu dikenali bila ada label "NIK". |
| Nomor KK | 16 digit dengan konteks "KK" / "Kartu Keluarga" |
| NPWP | format `99.999.999.9-999.999`, atau 15/16 digit dengan konteks "NPWP" |
| Nomor HP | seluler Indonesia `08…`, `62…`, `+62…`, dengan atau tanpa pemisah |
| Rekening | 7–18 digit dengan konteks: "rek", "a.n.", "VA", nama bank (BCA, BRI, BNI, Mandiri, BSI, Jago, SeaBank, …) |
| Kartu | 13–19 digit, lolos Luhn, prefiks jaringan kartu yang dikenal |
| BPJS | 13 digit dengan konteks "BPJS" / "JKN" / "KIS" |
| Email | alamat email |
| Nama, alamat, lahir | isian berlabel KTP/form (`Nama :`, `Alamat :`, `RT/RW`, `Kel/Desa`, `Kecamatan`, `Tempat/Tgl Lahir`), key JSON/`key=value` (`nama`, `full_name`, `alamat`, `dob`, …), dan nama setelah "a.n." |

Deteksi berbasis aturan sehingga mudah diaudit. Angka yang ambigu hanya dilaporkan bila ada kata kunci sebelumnya pada baris yang sama. Hash, UUID, nominal "Rp 1.500.000", timestamp, dan nomor pesanan dijaga agar tidak salah tangkap (lihat `TestNoFalsePositives`).

Keterbatasan: telepon rumah, nomor paspor, dan PDF belum didukung. Nama tanpa label tidak dikenali, termasuk kolom `nama` di CSV karena labelnya hanya ada di baris header. Untuk kasus ini, samarkan kolom tersebut di sumber datanya.

## Mengembangkan

```bash
go test -race ./...
go test ./pii -run '^$' -fuzz FuzzRedact -fuzztime 1m

./scripts/build-web.sh                 # build web ke dist/ (butuh Go + npm)
python3 -m http.server -d dist 8000    # buka http://localhost:8000
```

Semua data contoh di repo ini **fiktif**. Jangan pernah menambahkan data orang sungguhan ke tes atau issue. Lihat [CONTRIBUTING.md](CONTRIBUTING.md).

## Lisensi

[MIT](LICENSE) © 2026 Emil Rahmat. Web app menyertakan [tesseract.js](https://github.com/naptha/tesseract.js) dan data bahasa Indonesia (Apache-2.0).
