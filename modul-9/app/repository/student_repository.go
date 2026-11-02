package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"api-students/app/model"
	"api-students/helper"
)

var (
	ErrNotFound  = errors.New("data tidak ditemukan")
	ErrDuplicate = errors.New("data sudah ada")
)

var sortWhitelist = map[string]string{
	"id":    "id",
	"nim":   "nim",
	"name":  "name",
	"grade": "grade",
}

type StudentRepository interface {
	FindAll(ctx context.Context, q model.ListQuery) ([]model.Student, int, error)
	FindAfterCursor(ctx context.Context, q model.CursorQuery) ([]model.Student, model.CursorMeta, error)
	FindByID(ctx context.Context, id int) (model.Student, error)
	Create(ctx context.Context, s model.Student) (model.Student, error)
	Update(ctx context.Context, s model.Student) (model.Student, error)
	Delete(ctx context.Context, id int) error
}

type studentPostgresRepository struct {
	pool *pgxpool.Pool
}

func NewStudentRepository(pool *pgxpool.Pool) StudentRepository {
	return &studentPostgresRepository{pool: pool}
}

func buildFilter(q model.ListQuery) (string, []any) {
	where := " WHERE 1 = 1"
	args := []any{}

	if q.Search != "" {
		where += fmt.Sprintf(" AND name ILIKE $%d", len(args)+1)
		args = append(args, "%"+q.Search+"%")
	}
	if q.IsActive != nil {
		where += fmt.Sprintf(" AND is_active = $%d", len(args)+1)
		args = append(args, *q.IsActive)
	}
	if q.GradeMin != nil {
		where += fmt.Sprintf(" AND grade >= $%d", len(args)+1)
		args = append(args, *q.GradeMin)
	}
	if q.GradeMax != nil {
		where += fmt.Sprintf(" AND grade <= $%d", len(args)+1)
		args = append(args, *q.GradeMax)
	}
	return where, args
}

func (r *studentPostgresRepository) FindAll(
	ctx context.Context, q model.ListQuery,
) ([]model.Student, int, error) {
	where, args := buildFilter(q)

	var total int
	err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM students"+where, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("menghitung student: %w", err)
	}

	arah := "ASC"
	if q.Order == "desc" {
		arah = "DESC"
	}
	sortCol, ok := sortWhitelist[q.Sort]
	if !ok {
		sortCol = "id"
	}

	sqlText := fmt.Sprintf(
		`SELECT id, nim, name, grade, is_active, COALESCE(owner_id, 0), created_at
		 FROM students%s
		 ORDER BY %s %s
		 LIMIT $%d OFFSET $%d`,
		where, sortCol, arah, len(args)+1, len(args)+2,
	)
	args = append(args, q.Limit, q.Offset())

	rows, err := r.pool.Query(ctx, sqlText, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("mengambil daftar student: %w", err)
	}
	defer rows.Close() // WAJIB -- kalau lupa, koneksi tidak pernah kembali ke pool

	hasil := []model.Student{}
	for rows.Next() {
		var s model.Student
		if err := rows.Scan(
			&s.ID,
			&s.NIM,
			&s.Name,
			&s.Grade,
			&s.IsActive,
			&s.OwnerID,
			&s.CreatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("membaca baris student: %w", err)
		}
		hasil = append(hasil, s)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("membaca hasil query: %w", err)
	}

	return hasil, total, nil
}

func (r *studentPostgresRepository) FindByID(
	ctx context.Context, id int,
) (model.Student, error) {
	var s model.Student
	err := r.pool.QueryRow(ctx,
		`SELECT id, nim, name, grade, is_active, COALESCE(owner_id, 0), created_at
		 FROM students WHERE id = $1`, id,
	).Scan(&s.ID, &s.NIM, &s.Name, &s.Grade, &s.IsActive, &s.OwnerID, &s.CreatedAt)
	if err != nil {

		if errors.Is(err, pgx.ErrNoRows) {
			return model.Student{}, ErrNotFound
		}
		return model.Student{}, fmt.Errorf("mengambil student: %w", err)
	}
	return s, nil
}

func (r *studentPostgresRepository) Create(
	ctx context.Context, s model.Student,
) (model.Student, error) {

	err := r.pool.QueryRow(ctx,
		`INSERT INTO students (nim, name, grade, owner_id, is_active)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, created_at`,
		s.NIM, s.Name, s.Grade, s.OwnerID, s.IsActive,
	).Scan(&s.ID, &s.CreatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return model.Student{}, ErrDuplicate
		}
		return model.Student{}, fmt.Errorf("menyimpan student: %w", err)
	}
	return s, nil
}

func (r *studentPostgresRepository) Update(
	ctx context.Context, s model.Student,
) (model.Student, error) {
	err := r.pool.QueryRow(ctx,
		`UPDATE students SET nim = $1, name = $2, grade = $3, is_active = $4
		 WHERE id = $5
		 RETURNING id, nim, name, grade, COALESCE(owner_id, 0), is_active, created_at`,
		s.NIM, s.Name, s.Grade, s.IsActive, s.ID,
	).Scan(&s.ID, &s.NIM, &s.Name, &s.Grade, &s.OwnerID, &s.IsActive, &s.CreatedAt)
	if err != nil {
		// Tidak ada baris yang dikembalikan RETURNING berarti id-nya
		// memang tidak ada di tabel.
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Student{}, ErrNotFound
		}
		if isUniqueViolation(err) {
			return model.Student{}, ErrDuplicate
		}
		return model.Student{}, fmt.Errorf("memperbarui student: %w", err)
	}
	return s, nil
}

func (r *studentPostgresRepository) Delete(ctx context.Context, id int) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM students WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("menghapus student: %w", err)
	}
	// Perintah berhasil dijalankan, tetapi tidak ada baris yang terkena --
	// artinya id-nya memang tidak ada.
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// FindAfterCursor mengambil daftar student menggunakan cursor pagination.
// Pasangan column ordering: (created_at DESC, id DESC)
// Ini menjamin urutan unik dan konsisten.
//
// Algoritma:
// 1. Jika ada cursor, query: WHERE (created_at, id) < (cursor_created_at, cursor_id)
// 2. Ambil limit+1 baris
// 3. Jika hasil > limit, ada page berikutnya, ambil record terakhir untuk cursor
// 4. Return hasil[:limit] dan metadata cursor
func (r *studentPostgresRepository) FindAfterCursor(
	ctx context.Context, q model.CursorQuery,
) ([]model.Student, model.CursorMeta, error) {
	// Validasi limit
	limit := q.Limit
	if limit < 1 || limit > 100 {
		limit = 10
	}

	// Build WHERE clause untuk filter search/is_active
	where := ""
	args := []interface{}{}

	if q.Search != "" {
		where += " AND (LOWER(nim) LIKE LOWER($1) OR LOWER(name) LIKE LOWER($2))"
		pattern := "%" + q.Search + "%"
		args = append(args, pattern, pattern)
	}

	if q.IsActive != nil {
		argPos := len(args) + 1
		where += fmt.Sprintf(" AND is_active = $%d", argPos)
		args = append(args, *q.IsActive)
	}

	// Jika ada cursor, tambah kondisi keyset pagination
	// (created_at, id) < (cursor.CreatedAt, cursor.ID)
	if q.After != nil {
		argPos := len(args) + 1
		where += fmt.Sprintf(
			" AND (created_at, id) < ($%d::TIMESTAMPTZ, $%d::INT)",
			argPos, argPos+1,
		)
		args = append(args, q.After.CreatedAt, q.After.ID)
	}

	// Query dengan LIMIT+1 untuk deteksi apakah ada page berikutnya
	sqlText := fmt.Sprintf(
		`SELECT id, nim, name, grade, is_active, COALESCE(owner_id, 0), created_at
		 FROM students
		 WHERE TRUE%s
		 ORDER BY created_at DESC, id DESC
		 LIMIT $%d`,
		where, len(args)+1,
	)
	args = append(args, limit+1)

	rows, err := r.pool.Query(ctx, sqlText, args...)
	if err != nil {
		return nil, model.CursorMeta{}, fmt.Errorf("cursor pagination query: %w", err)
	}
	defer rows.Close()

	hasil := []model.Student{}
	for rows.Next() {
		var s model.Student
		if err := rows.Scan(
			&s.ID,
			&s.NIM,
			&s.Name,
			&s.Grade,
			&s.IsActive,
			&s.OwnerID,
			&s.CreatedAt,
		); err != nil {
			return nil, model.CursorMeta{}, fmt.Errorf("scan row: %w", err)
		}
		hasil = append(hasil, s)
	}
	if err := rows.Err(); err != nil {
		return nil, model.CursorMeta{}, fmt.Errorf("iterasi baris: %w", err)
	}

	// Jika hasil > limit, ada page berikutnya
	hasMore := len(hasil) > limit
	meta := model.CursorMeta{HasMore: hasMore}

	// Jika ada halaman berikutnya, ambil cursor dari record terakhir limit
	if hasMore {
		lastRecord := hasil[limit-1] // Ambil record di posisi limit-1 (0-indexed)
		meta.NextCursor = helper.EncodeCursor(lastRecord.CreatedAt, lastRecord.ID)
		hasil = hasil[:limit] // Potong ke limit
	}

	return hasil, meta, nil
}

// isUniqueViolation memeriksa apakah error berasal dari pelanggaran
// batasan UNIQUE. Kode 23505 adalah kode resmi PostgreSQL untuk itu.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}