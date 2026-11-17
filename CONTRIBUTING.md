# Kontribusi

Terima kasih sudah mau membantu! Kontribusi paling berharga:

- **Pola baru** (jenis data, label form, nama bank) beserta tesnya.
- **False positive/negative**: contoh teks yang salah dideteksi. Tolong gunakan **data fiktif** yang strukturnya serupa.
- Perbaikan web app (aksesibilitas, OCR, UX di HP).

## Aturan penting

1. **Jangan pernah mengirim data pribadi asli** (foto KTP, NIK, nomor HP orang lain) di issue, PR, atau tes. Buat data sintetis, misalnya NIK `3201014508900001`.
2. Setiap aturan deteksi baru wajib punya tes positif **dan** tes negatif di `pii/pii_test.go` (`TestScan`, `TestNoFalsePositives`).
3. Jalankan sebelum mengirim PR:

```bash
gofmt -l .
go vet ./...
go test -race ./...
go test ./pii -run '^$' -fuzz FuzzRedact -fuzztime 30s
```

4. Web app harus tetap berjalan sepenuhnya lokal: jangan menambah CDN, analytics, atau request ke domain lain.

## Alur kerja

1. Fork repo ini dan buat branch baru.
2. Buat perubahan yang fokus dan kecil.
3. Buka pull request dengan penjelasan singkat dan contoh input/output.
