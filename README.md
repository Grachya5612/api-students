# api-students — Dokumentasi API

REST API sederhana untuk mengelola data mahasiswa, dibangun pakai [Fiber](https://gofiber.io/) (Go).

**Base URL (lokal):** `http://localhost:3000`

## Amplop Respons

Seluruh endpoint — sukses maupun gagal — selalu mengembalikan bentuk JSON yang sama:

```json
{
  "success": true,
  "message": "Pesan singkat status operasi",
  "data": "isi data (opsional, tergantung endpoint)",
  "meta": "info tambahan seperti paginasi (opsional, cuma ada di GET list)"
}
```

Kalau gagal, `success` bernilai `false`, `data` biasanya `null` atau berisi rincian error (khusus 422), dan field `meta` tidak muncul.

## Kontrak API

| Metode | Endpoint | Parameter | Contoh Body Permintaan | Status yang Mungkin | Contoh Respons |
|---|---|---|---|---|---|
| **GET** | `/api/v1/students` | Query (semua opsional): `page` (default `1`), `limit` (default `10`, maks `100`), `search` (nama, tidak case-sensitive), `sort` (`id`\|`nim`\|`name`\|`grade`), `order` (`asc`\|`desc`), `is_active` (`true`\|`false`), `grade_min`, `grade_max` (angka) | — (tidak ada body) | `200` sukses · `400` nilai query tidak valid (mis. `sort` di luar whitelist, `is_active` bukan boolean) | `{"success":true,"message":"Berhasil mengambil daftar mahasiswa","data":[{"id":1,"nim":"2024001","name":"Cia","grade":3.45,"is_active":true}],"meta":{"page":1,"limit":10,"total":4,"total_pages":1}}` |
| **GET** | `/api/v1/students/:id` | Path: `id` (integer) | — (tidak ada body) | `200` sukses · `400` id bukan angka · `404` data tidak ditemukan | `{"success":true,"message":"Berhasil mengambil data mahasiswa","data":{"id":1,"nim":"2024001","name":"Cia","grade":3.45,"is_active":true}}` |
| **POST** | `/api/v1/students` | Header wajib: `Content-Type: application/json` | `{"nim":"2024100","name":"Rani","grade":3.6,"is_active":true}` (semua field wajib diisi) | `201` sukses (+ header `Location`) · `400` body bukan JSON yang sah · `409` NIM sudah dipakai · `415` Content-Type bukan `application/json` · `422` validasi field gagal | `{"success":true,"message":"Mahasiswa baru berhasil ditambahkan","data":{"id":5,"nim":"2024100","name":"Rani","grade":3.6,"is_active":true}}` |
| **PUT** | `/api/v1/students/:id` | Path: `id` (integer). Header wajib: `Content-Type: application/json` | `{"nim":"2024001","name":"Cia Updated","grade":4.0,"is_active":false}` (mengganti SELURUH field, semua wajib diisi) | `200` sukses · `400` id bukan angka / body bukan JSON · `404` data tidak ditemukan · `409` NIM sudah dipakai mahasiswa lain · `415` Content-Type salah · `422` validasi field gagal | `{"success":true,"message":"Data mahasiswa berhasil diperbarui","data":{"id":1,"nim":"2024001","name":"Cia Updated","grade":4,"is_active":false}}` |
| **PATCH** | `/api/v1/students/:id` | Path: `id` (integer). Header wajib: `Content-Type: application/json` | `{"grade":2.75}` (hanya field yang dikirim yang diperbarui, sisanya tidak berubah) | `200` sukses · `400` id bukan angka / body bukan JSON · `404` data tidak ditemukan · `409` NIM (jika dikirim) sudah dipakai mahasiswa lain · `415` Content-Type salah · `422` validasi field gagal | `{"success":true,"message":"Data mahasiswa berhasil diperbarui sebagian","data":{"id":1,"nim":"2024001","name":"Cia Updated","grade":2.75,"is_active":false}}` |
| **DELETE** | `/api/v1/students/:id` | Path: `id` (integer) | — (tidak ada body) | `204` sukses (tanpa body) · `400` id bukan angka · `404` data tidak ditemukan | *(kosong, hanya status `204 No Content`)* |

## Contoh Respons Gagal (per Status Code)

Supaya lebih jelas, berikut contoh nyata tiap status error, hasil pengujian langsung:

**400 — id bukan angka**
```json
{"success":false,"message":"ID harus berupa angka"}
```

**400 — body bukan JSON yang sah**
```json
{"success":false,"message":"Body bukan JSON yang sah"}
```

**404 — data tidak ditemukan**
```json
{"success":false,"message":"Mahasiswa dengan ID tersebut tidak ditemukan"}
```

**409 — NIM sudah dipakai (konflik)**
```json
{"success":false,"message":"NIM sudah terdaftar pada mahasiswa lain"}
```

**415 — Content-Type bukan application/json**
```json
{"success":false,"message":"Content-Type harus application/json"}
```

**422 — validasi field gagal (rincian per field)**
```json
{
  "success": false,
  "message": "Validasi gagal, periksa kembali field yang dikirim",
  "data": {
    "errors": [
      {"field": "name", "message": "Name wajib diisi"},
      {"field": "grade", "message": "Grade harus di antara 0 dan 4"}
    ]
  }
}
```

## Query String pada GET /api/v1/students (Detail)

| Param | Tipe | Default | Keterangan |
|---|---|---|---|
| `page` | integer | `1` | Halaman ke berapa |
| `limit` | integer | `10` | Jumlah data per halaman, maksimal `100` (dibatasi supaya server tidak dipaksa mengirim payload raksasa dalam satu request) |
| `search` | string | – | Mencari substring pada `name`, tidak membedakan huruf besar/kecil |
| `sort` | string | `id` | Field pengurutan. Whitelist: `id`, `nim`, `name`, `grade` |
| `order` | string | `asc` | `asc` atau `desc` |
| `is_active` | boolean | – | Filter berdasarkan status aktif |
| `grade_min` | number | – | Filter nilai minimum (inklusif) |
| `grade_max` | number | – | Filter nilai maksimum (inklusif) |

## Menjalankan Secara Lokal

```bash
go mod tidy
go run .
```

Server berjalan di `http://localhost:3000`.
