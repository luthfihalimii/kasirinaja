# KasirinAja POS

KasirinAja adalah web POS (Point of Sale) untuk minimarket yang dibuat supaya proses kasir lebih cepat, rapi, dan mudah dipantau.
Bahasanya sederhana: aplikasi ini membantu toko mencatat transaksi, mengelola stok, memantau tim kasir, dan menyiapkan pembelian barang tanpa spreadsheet manual.

## Apa Yang Bisa Dilakukan

Fitur kasir harian:
- Checkout cepat dengan hitung subtotal, diskon, pajak, dan kembalian.
- Hold cart (tahan keranjang) saat pelanggan belum jadi bayar.
- Split payment (misalnya sebagian cash, sebagian QR/debit).
- Shift kasir buka/tutup untuk kontrol kas harian.

Fitur admin:
- Tambah dan kelola produk.
- Tambah akun kasir baru.
- Kelola promo toko.
- Laporan harian dan audit log aktivitas.
- Alert anomali operasional (void/refund/aktivitas tidak biasa).

Fitur stok dan pembelian:
- Kelola supplier.
- Buat Purchase Order (PO) dan receive PO.
- Update stok otomatis setelah penerimaan barang.
- Simpan HPP/cost produk untuk analitik margin dan reorder suggestion.

Fitur AI operasional:
- Smart recommendation untuk upsell saat checkout.
- Attach-rate metrics untuk evaluasi performa rekomendasi.

## Stack Teknologi

- Frontend: Next.js + React + komponen UI style shadcn.
- Backend API: Go.
- Database utama: PostgreSQL.
- Cache: Redis (opsional, sistem tetap jalan tanpa Redis).

## Arsitektur Singkat

- Frontend (`frontend`) berbicara ke backend HTTP API.
- Backend (`backend`) mengelola auth, bisnis proses POS, dan persistence.
- PostgreSQL menyimpan data transaksi, produk, pengguna, stok, PO, audit, dan metrik.
- Redis dipakai untuk cache recommendation (jika tersedia).

## Prasyarat

- Bun (untuk frontend, sesuai preferensi project ini).
- Go `1.25` (sesuai `backend/go.mod`).
- PostgreSQL aktif di lokal.
- Redis opsional.

## Menjalankan Project (Local Development)

### 1) Siapkan database dan migration

Contoh jika database bernama `kasirinaja` sudah ada:

```bash
cd backend
psql -U postgres -d kasirinaja -f migrations/001_init.sql
psql -U postgres -d kasirinaja -f migrations/002_pos_hardening.sql
psql -U postgres -d kasirinaja -f migrations/003_admin_reporting.sql
psql -U postgres -d kasirinaja -f migrations/004_persistence_upgrade.sql
psql -U postgres -d kasirinaja -f migrations/005_shift_promo_hardening.sql
```

### 2) Jalankan backend

```bash
cd backend
DATABASE_URL='postgres://postgres@127.0.0.1:5432/kasirinaja?sslmode=disable' \
AUTH_SECRET='replace-with-random-32-char-secret' \
MANAGER_PIN='739154' \
PORT=8080 \
go run ./cmd/server
```

Backend default: `http://127.0.0.1:8080`

### 3) Jalankan frontend (Bun)

```bash
cd frontend
bun install
NEXT_PUBLIC_API_BASE_URL=http://127.0.0.1:8080 bun run dev -- --hostname 127.0.0.1 --port 3000
```

Frontend default: `http://127.0.0.1:3000`

### 4) Login awal

Sejak migration `004` versi terbaru, sistem tidak lagi membuat akun default otomatis.
Untuk bootstrap admin pertama kali, buat akun admin manual:

```bash
psql -U postgres -d kasirinaja -c "INSERT INTO app_users (username, password, role, active) VALUES ('admin', 'ganti-password-kuat', 'admin', true) ON CONFLICT (username) DO NOTHING;"
```

Lalu login dengan akun itu dari UI/API. Password plaintext bootstrap akan di-upgrade otomatis menjadi hash bcrypt saat proses login pertama berhasil.

Jika kamu pakai database lama yang dulu pernah menjalankan migration `004` versi lama, akun `admin/admin123` dan `cashier/cashier123` mungkin masih ada. Disarankan ganti password segera.

Manager PIN untuk aksi sensitif harus diset via environment (`MANAGER_PIN`) dengan nilai kuat.

## Alur Cepat Untuk Orang Awam

1. Login sebagai admin.
2. Buka menu produk, lalu tambah produk yang dijual.
3. Buka menu tim kasir, lalu tambah akun kasir baru.
4. Kasir login dan buka shift.
5. Mulai transaksi di menu kasir.
6. Gunakan hold cart jika pelanggan menunda pembayaran.
7. Gunakan menu procurement untuk supplier dan PO saat stok menipis.
8. Cek laporan harian dan alert operasional dari dashboard admin.

## API Inti

- `POST /api/v1/auth/login`
- `GET|POST /api/v1/products`
- `POST /api/v1/checkout`
- `GET|POST /api/v1/carts/hold`
- `GET|POST /api/v1/suppliers`
- `GET|POST /api/v1/purchase-orders`
- `GET|POST /api/v1/users/cashiers`
- `GET /api/v1/reports/daily`
- `GET /api/v1/alerts/anomalies`

## Konfigurasi Environment Penting (Backend)

- `PORT` (default: `8080`)
- `ALLOWED_ORIGIN` (default: `http://127.0.0.1:3000`)
- `DATABASE_URL` (kosong = fallback in-memory)
- `REDIS_ADDR` (opsional)
- `DEFAULT_STORE_ID` (default: `main-store`)
- `AUTH_SECRET` (wajib diisi, min 32 karakter)
- `ACCESS_TOKEN_TTL_MINUTES` (default: `480`)
- `MANAGER_PIN` (wajib diisi, min 6 digit dan tidak boleh PIN lemah)

## Struktur Folder

- `backend/` layanan API, domain logic, store (memory + postgres), migration SQL.
- `frontend/` aplikasi dashboard POS.
- `docs/` catatan tambahan proyek.

## Catatan

- Docker Compose tersedia untuk setup cepat service dasar.
- Saat development lokal, workflow utama di proyek ini menggunakan Bun untuk frontend.
- Docker Compose saat ini sudah memuat migration `001` sampai `005` untuk inisialisasi database baru.
- Jika `DATABASE_URL` di-set tapi PostgreSQL tidak bisa diakses, backend akan gagal start (fail-fast) agar tidak diam-diam fallback ke mode in-memory.
