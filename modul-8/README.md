# api-students — REST API Mahasiswa (PostgreSQL + Repository Pattern)

REST API untuk data mahasiswa. Sejak revisi ini, data tersimpan permanen di **PostgreSQL**
lewat pola **repository** (bukan lagi slice di memori).

## Struktur Proyek

```
api-students/
├── app/
│   ├── model/
│   │   └── student.go          struct entitas, request, respons, ListQuery
│   └── repository/
│       └── student_repository.go   kontrak (interface) & implementasi PostgreSQL
├── config/
│   └── env.go                  memuat variabel environment dari .env
├── database/
│   └── postgres.go             connection pool ke PostgreSQL + Ping
├── migrations/
│   └── 001_create_students.sql skema tabel students
├── .env                        konfigurasi lokal (JANGAN di-commit, sudah di .gitignore)
├── .env.example                daftar variabel yang perlu diisi (aman di-commit)
├── main.go                     perakitan pool → repository → handler, routing
├── handler.go                  logika tiap endpoint (memakai repository)
└── helper.go                   response helper, parsing query, middleware
```

## Skema Tabel `students`

| Kolom | Tipe | Keterangan |
|---|---|---|
| `id` | `SERIAL PRIMARY KEY` | ID internal, auto-increment |
| `nim` | `VARCHAR(20) NOT NULL` | Nomor Induk Mahasiswa, **unik** (lihat indeks di bawah) |
| `name` | `VARCHAR(100) NOT NULL` | Nama mahasiswa |
| `grade` | `NUMERIC(3,2) NOT NULL DEFAULT 0` | Nilai/IPK, 0.00–4.00 |
| `is_active` | `BOOLEAN NOT NULL DEFAULT TRUE` | Status keaktifan |
| `created_at` | `TIMESTAMPTZ NOT NULL DEFAULT NOW()` | Waktu data dibuat |

**Indeks:**
- `students_nim_lower_key` — `UNIQUE INDEX` pada `LOWER(nim)`. Menjaga keunikan NIM tanpa
  membedakan huruf besar/kecil, **di level basis data**, bukan cuma di kode Go. Ini penting
  karena basis data yang memutuskan siapa lebih dulu kalau dua request datang nyaris
  bersamaan (race condition) — pengecekan manual di Go ("SELECT dulu, baru INSERT") selalu
  punya celah waktu di antara dua langkah itu.
- `students_grade_idx` — indeks B-tree biasa pada `grade`. Dipakai untuk mempercepat filter
  rentang nilai (`grade_min`, `grade_max`) dan pengurutan (`?sort=grade`), supaya PostgreSQL
  tidak perlu memindai seluruh tabel (*sequential scan*) tiap kali query semacam itu dijalankan.

## Cara Setup dari Nol

Asumsikan pembaca baru saja meng-*clone* repositori ini dan **belum punya apa-apa** selain Go dan PostgreSQL ter-install.

**1. Buat database kosong**
```bash
psql -U postgres -c "CREATE DATABASE praktikum_backend;"
```

**2. Jalankan migrasi**
```bash
psql -U postgres -d praktikum_backend -f migrations/001_create_students.sql
```

**3. Salin dan isi berkas environment**
```bash
cp .env.example .env
```
Buka `.env`, isi sesuai kredensial PostgreSQL di komputer kamu (lihat tabel variabel di bawah).

**4. Install dependency & jalankan**
```bash
go mod tidy
go run .
```

Server berjalan di `http://localhost:3000` (atau sesuai `APP_PORT` di `.env`).

**5. Verifikasi**
```bash
curl http://localhost:3000/api/v1/health
```
Kalau hasilnya `{"success":true,"message":"server dan database berjalan"}`, setup sudah benar.

## Variabel Environment

| Variabel | Wajib? | Contoh | Keterangan |
|---|---|---|---|
| `APP_PORT` | Tidak (default `3000`) | `3000` | Port server Fiber |
| `DB_HOST` | Ya | `localhost` | Host PostgreSQL |
| `DB_PORT` | Tidak (default `5432`) | `5432` | Port PostgreSQL |
| `DB_USER` | Ya | `postgres` | Username PostgreSQL |
| `DB_PASSWORD` | Ya | `rahasia123` | Password PostgreSQL — **jangan pernah commit nilai asli ini** |
| `DB_NAME` | Ya | `praktikum_backend` | Nama database |
| `DB_SSLMODE` | Tidak (default `disable`) | `disable` | `disable` untuk lokal; pakai `require` di server produksi |
| `DB_MAX_CONNS` | Tidak (default `10`) | `10` | Jumlah maksimum koneksi dalam connection pool |

Berkas `.env` **tidak ikut ter-commit** (sudah masuk `.gitignore`). Yang di-commit hanya
`.env.example` berisi nama variabel dengan nilai kosong, supaya rekan yang meng-*clone*
tahu variabel apa saja yang perlu diisi tanpa pernah melihat kredensial asli.

## Endpoint

Base path: `/api/v1`

| Metode | Endpoint | Keterangan |
|---|---|---|
| GET | `/health` | Cek kesehatan server **dan** koneksi database |
| GET | `/students` | Daftar mahasiswa (paginasi, search, sort, filter — lihat query string) |
| GET | `/students/:id` | Satu mahasiswa |
| POST | `/students` | Tambah mahasiswa baru |
| PUT | `/students/:id` | Ganti seluruh data (semua field wajib) |
| PATCH | `/students/:id` | Ubah sebagian data (hanya field yang dikirim) |
| DELETE | `/students/:id` | Hapus mahasiswa |

### Query String pada GET /students

| Param | Default | Keterangan |
|---|---|---|
| `page` | `1` | Halaman ke berapa |
| `limit` | `10` | Baris per halaman, maksimal `100` |
| `search` | – | Cari substring pada `name`, dieksekusi via `ILIKE` (tidak case-sensitive) |
| `sort` | `id` | Whitelist: `id`, `nim`, `name`, `grade` |
| `order` | `asc` | `asc` atau `desc` |
| `is_active` | – | `true` / `false` |
| `grade_min`, `grade_max` | – | Rentang nilai, inklusif |

### Status HTTP yang Dikembalikan

| Status | Situasi |
|---|---|
| 200 | Berhasil ambil / ubah data |
| 201 | Berhasil tambah data (+ header `Location`) |
| 204 | Berhasil hapus data (tanpa body) |
| 400 | id bukan angka, body bukan JSON valid, atau PATCH tanpa field apa pun |
| 404 | Data tidak ditemukan (`repository.ErrNotFound` dari `pgx.ErrNoRows` atau `RowsAffected == 0`) |
| 409 | NIM sudah dipakai mahasiswa lain (`repository.ErrDuplicate` dari kode PostgreSQL `23505`) |
| 415 | Content-Type request bukan `application/json` |
| 422 | Validasi field gagal (rincian per field di `data.errors`) |
| 500 | Error tak terduga dari database saat memproses request CRUD |
| 503 | `GET /health` gagal melakukan `Ping()` ke database |

**Catatan soal 500 vs 503 saat database mati:** endpoint `/health` secara eksplisit
memanggil `pool.Ping()` dan mengembalikan **503 Service Unavailable** — status yang secara
semantik memang berarti "server hidup, tapi layanan di baliknya sedang tidak tersedia".
Endpoint CRUD lain (`/students`, dst) tidak melakukan pengecekan khusus; begitu query gagal
karena database mati, error itu jatuh ke jalur default dan dikembalikan sebagai
**500 Internal Server Error** — status generik untuk "server gagal memproses request karena
alasan di sisi server". Keduanya sudah teruji: mematikan PostgreSQL lalu memanggil `/health`
menghasilkan 503, sedangkan memanggil `/students` pada kondisi yang sama menghasilkan 500.
