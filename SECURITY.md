# Kebijakan Keamanan

## Melaporkan kerentanan

Laporkan secara privat lewat [GitHub Security Advisories](https://github.com/Emil-02/sensor/security/advisories/new). Jangan membuka issue publik. Laporan akan ditanggapi secepatnya.

Contoh hal yang termasuk masalah keamanan:

- Web app mengirim foto/teks ke server mana pun.
- Data yang terdeteksi tetap terbaca di hasil ekspor (misalnya kotak penutup tidak ikut tergambar, atau metadata tersisa).
- Input yang membuat proses hang atau crash (ReDoS, panic).

Data yang **tidak terdeteksi** (false negative) biasanya bukan kerentanan. Silakan buka issue biasa dengan contoh data **fiktif**.

## Desain keamanan

- Web app berjalan sepenuhnya di browser. CSP membatasi koneksi ke origin sendiri, dan semua aset (WASM, OCR, data bahasa) di-host sendiri dengan versi yang dipin.
- Penutup gambar memakai warna solid, bukan blur/pixelate yang bisa dipulihkan. Ekspor PNG lewat canvas membuang metadata EXIF/GPS.
- Mode `hash` memakai HMAC-SHA256 dengan kunci rahasia, sehingga nomor tidak bisa ditebak dengan brute force tanpa kunci.
- Regex Go (RE2) berjalan dalam waktu linear, jadi tidak rentan ReDoS. Panjang input diuji dengan fuzzing.
