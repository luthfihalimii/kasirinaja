# KasirinAja Backend (Go)

Backend API untuk POS minimarket dengan fitur Smart Basket AI.

## Fitur API

- `GET /healthz`
- `GET /api/v1/products`
- `POST /api/v1/cart/recommendation`
- `POST /api/v1/checkout`
- `POST /api/v1/sync/offline-transactions`
- `GET /api/v1/metrics/attach-rate?store_id=main-store&days=30`

## Jalankan lokal

```bash
cd backend
cp .env.example .env
go run ./cmd/server
```

## Mode Penyimpanan

- Jika `DATABASE_URL` valid dan server PostgreSQL aktif, backend otomatis pakai PostgreSQL.
- Jika `DATABASE_URL` di-set tapi koneksi database gagal, backend akan fail-fast saat startup.
- Jika `DATABASE_URL` kosong, backend berjalan dalam mode in-memory untuk dev/demo.
- Schema PostgreSQL disiapkan di `migrations/001_init.sql`.

## Redis

- Jika `REDIS_ADDR` valid, recommendation response akan dicache di Redis.
- Jika Redis tidak tersedia, backend fallback ke `NoopRecommendationCache`.

## Migrations

SQL schema ada di `migrations/001_init.sql`.
