# DESIGN.md — TempMail By JhopanStore

> **Status: arah ditulis agent, dial dikonfirmasi pemilik (2026-11-12).** Pemilik produk belum menulis
> arahnya sendiri, jadi dokumen ini saya susun dari bukti di repo (palet dan tipografi panel yang sudah
> jalan, cara pakai di server, dan batasan "tanpa CDN"). Peringatan jujur: selera yang saya tulis
> cenderung ke rasa AI default dan bisa terasa monoton. Dial dan aturan "Jangan" sudah disetujui; bagian
> identitas dan karakter tetap draft yang menunggu kamu rapikan.

## Siapa yang memakainya

Operator tunggal (pemilik server) yang mengurus domain temp mail, Worker, tunnel, dan kondisi server,
sering dari HP lewat tunnel pribadi, kadang lewat SSH. Ini alat kerja, bukan halaman promosi: yang
dibutuhkan cepat terbaca, bukan mengesankan.

## Karakter

Tenang, padat data, tanpa dekorasi. Satu layar = satu pekerjaan. Kejutan visual tidak diinginkan:
warna hanya dipakai untuk menandai status dan hierarki, bukan untuk hiasan.

## Palet (maksimal 2 netral + 1 aksen)

| Peran | Nilai | Alasan satu baris |
|---|---|---|
| Latar | `#0a0a0f` | Gelap pekat supaya panel terbaca nyaman malam hari saat kerja panjang, sekaligus menyatu dengan UI inbox yang sudah ada |
| Permukaan | `#12121a`, `#171722` | Dua tingkat saja untuk memisahkan kartu dari latarnya dan baris aktif dari baris biasa |
| Garis | `#1e1e2e` | Pemisah tipis, tidak menarik perhatian |
| Teks | `#e2e8f0`, `#94a3b8` | Teks utama kontras tinggi; teks sekunder dinaikkan ke `#94a3b8` supaya lolos WCAG AA (7.3:1 di permukaan, 7.7:1 di latar) |
| Aksen | `#8b5cf6` (tekan: `#7c3aed`) | Identitas produk yang sudah dipakai UI inbox; dipakai hemat: focus ring, tombol utama, penanda nav aktif |
| Status | `#22c55e`, `#ef4444`, `#3b82f6` | Hanya untuk keadaan nyata (aktif/off, error, tautan), tidak untuk dekorasi |

Aturan: satu aksen saja. Status bukan aksen. Tanpa gradient latar, tanpa glow, tanpa orb.

## Tipografi

- **Sans sistem** (`-apple-system`, `Segoe UI`, `Roboto`, `sans-serif`): panel dijalankan di OS apa pun
  dan repo melarang CDN, jadi tidak ada webfont; font sistem juga paling cepat dan tidak berkedip.
- **Monospace** (`ui-monospace`, `SF Mono`, `Menlo`) hanya untuk nilai teknis yang harus persis:
  token, path, zone id, nama Worker, keluaran log. Bukan untuk judul.
- Ukuran: judul panel 15px, label 11-12px, isi 13px, nilai teknis 12px. Berat 600-700 hanya untuk judul
  dan tombol, sisanya 400.

## Bentuk dan susunan

- Sidebar tetap 252px (drawer 278px di mobile). Kolom isi **selebar layar** tanpa batas maksimum:
  panel ini dibaca di layar lebar (tabel domain, log, keluaran helper), dan ruang kosong di kanan
  tidak memberi nilai. Sisa lebar hanya untuk padding. Tetap utuh dari 1360px sampai 360px.
  (Diperbarui 2026-11-12: keputusan pemilik, batas 1100px lama dihapus.)
- Radius 8-10px konsisten; `999px` hanya untuk badge status, supaya bentuk itu tetap punya arti.
- Kartu untuk satu blok data, tabel untuk daftar yang bisa tumbuh, `dl` (kv) untuk fakta pasangan
  label-nilai, `pre` untuk keluaran panjang. Tiap seksi memakai bentuk yang paling cocok, bukan satu
  pola untuk semua.
- Kepadatan: padding 14-15px di kartu, 9-11px di sel tabel, jarak antar blok 14px. Konsisten supaya mata
  tahu ke mana harus melompat.
- Aksen kiri 2px pada item nav aktif: penanda posisi, bukan hiasan kartu.

### Papan status (Dashboard)

Tiap bentuk punya alasan satu baris, bukan dekorasi:

| Bentuk | Menjawab pertanyaan | Alasan |
|---|---|---|
| Cincin kesiapan | "berapa banyak yang siap" | persentase terbaca lebih cepat sebagai busur daripada sebagai angka |
| Alur email (6 simpul berwarna) | "kalau email tidak masuk, putus di mana" | urutan nyata jalur surat, bukan diagram hiasan |
| Grafik 24 jam | "ada aktivitas atau tidak" | pola jam terlihat, angka total saja tidak menunjukkan waktu |
| Ubin angka (inbox, database, RAM) | "seberapa besar dan seberapa penuh" | angka besar + bar RAM terhadap target 100 MB |
| Kisi titik aspek | "komponen mana yang merah" | delapan komponen dalam satu pandangan, warna = status nyata |
| Kartu aksi (ikon + label + baris kecil) | "apa yang bisa saya lakukan" | target sentuh besar, aksi merusak diberi warna merah |
| Chip panjang log (50/100/200/500) | "berapa banyak log" | satu nilai aktif dari empat, lebih ringkas daripada select |
| Dot grid fakta host (tunnel/unit/domain) | "bagaimana keadaan sebenarnya di server" | satu titik per komponen, nilainya teks kecil |

- Ikon pada kartu aksi hanya bentuk yang punya arti langsung: `◍` status, `⤓` backup/salinan keluar,
  `✚` periksa/doctor, `⟳` restart. Tidak ada ikon hiasan, tidak ada panah di tiap tombol. Glif dipakai
  karena tiga dari empat aksi adalah operasi sistem (bukan konsep produk) dan teksnya sudah menjelaskan;
  glif hanya pemandum cepat, bukan pengganti label.

## Gerak

Hover dan focus 150-180ms, drawer 180ms, spinner hanya saat memuat data. Tanpa animasi masuk-masuk,
tanpa parallax. `prefers-reduced-motion` mematikan transisi.

## Motif identitas

1. **Pill status** di header (mis. `helper: terpasang`) sebagai jawaban pertama atas pertanyaan
   "apakah semuanya hidup".
2. **Label teknis berbahasa Indonesia** yang menyebut tindakan nyata (`Simpan token`, `Enable routing`),
   bukan istilah pemasaran.
3. **Titik status** (kisi `dotgrid`): satu titik per komponen, hijau siap, kuning perlu dicek, merah
   bermasalah. Muncul identik di Dashboard, Diagnostik, Server, dan Tunnel supaya satu warna selalu
   berarti hal yang sama di seluruh panel.

## Fokus dan papan ketik

- `:focus-visible` = garis 2px warna aksen + offset 2px, di semua kontrol (termasuk tombol nav, chip,
  kartu aksi). Tidak ada `outline: none` tanpa pengganti.
- Urutan Tab mengikuti urutan visual: sidebar dulu (11 panel), lalu isi panel, lalu kaki sidebar.
- Modal: fokus masuk ke tombol tutup saat dibuka, `Escape` menutup, fokus kembali ke tombol pemicu.
- Di layar ≤640px semua kontrol yang bisa disentuh minimal 44px tinggi (`.btn`, `.chip`, `.action-tile`,
  `input`, `select`, `.nav-item`).

## Jangan

- Gradient latar, glow, glassmorphism, ikon dekoratif (sparkle/star/lightning), panah di tiap tombol.
- Angka atau klaim tanpa sumber; data yang tampil selalu berasal dari server.
- Tema terang: panel ini alat operator, bukan pilihan gaya pengguna (keputusan produk, bukan default).
- Meniru tampilan produk lain (Linear, Vercel, Stripe) secara menyeluruh.

## Dial

`Dial: ENERGY 2 / RHYTHM 2 / MOTION 1` (dikonfirmasi pemilik, 2026-11-12)

- ENERGY 2: hierarki jelas dan ada aksen brand, tapi tanpa teriakan.
- RHYTHM 2: kv, tabel, dan form tampil berbeda sesuai isinya.
- MOTION 1: hampir statis, gerak hanya untuk umpan balik.
