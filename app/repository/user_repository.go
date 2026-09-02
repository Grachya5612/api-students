package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"api-students/app/model"
)

type UserRepository interface {
	FindAll(ctx context.Context, q model.ListQuery) ([]model.User, int, error)
	FindByID(ctx context.Context, id int) (model.User, error)
	Create(ctx context.Context, user model.User) (model.User, error)
	Update(ctx context.Context, user model.User) (model.User, error)
	Delete(ctx context.Context, id int) error
}

type userRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) UserRepository {
	return &userRepository{pool: pool}
}

// FindAll mengambil daftar user beserta jumlah total data.
func (r *userRepository) FindAll(
	ctx context.Context,
	q model.ListQuery,
) ([]model.User, int, error) {

	where := " WHERE 1 = 1"
	args := []any{}

	if q.Search != "" {
		args = append(args, "%"+q.Search+"%")
		where += fmt.Sprintf(
			" AND (username ILIKE $%d OR email ILIKE $%d)",
			len(args),
			len(args),
		)
	}

	if q.IsActive != nil {
		args = append(args, *q.IsActive)
		where += fmt.Sprintf(" AND is_active = $%d", len(args))
	}

	if q.GradeMin != nil {
		// GradeMin tidak digunakan karena tabel users
		// tidak mempunyai kolom grade.
	}

	if q.GradeMax != nil {
		// GradeMax tidak digunakan karena tabel users
		// tidak mempunyai kolom grade.
	}

	allowedSort := map[string]string{
		"id":         "id",
		"username":   "username",
		"email":      "email",
		"is_active":  "is_active",
		"created_at": "created_at",
	}

	sortColumn := allowedSort[q.Sort]
	if sortColumn == "" {
		sortColumn = "id"
	}

	order := "ASC"
	if q.Order == "desc" {
		order = "DESC"
	}

	countQuery := "SELECT COUNT(*) FROM users" + where

	var total int
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := fmt.Sprintf(`
		SELECT id, username, email, password, is_active, created_at
		FROM users
		%s
		ORDER BY %s %s
		LIMIT $%d OFFSET $%d
	`,
		where,
		sortColumn,
		order,
		len(args)+1,
		len(args)+2,
	)

	args = append(args, q.Limit, q.Offset())

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	users := make([]model.User, 0)

	for rows.Next() {
		var user model.User

		if err := rows.Scan(
			&user.ID,
			&user.Username,
			&user.Email,
			&user.Password,
			&user.IsActive,
			&user.CreatedAt,
		); err != nil {
			return nil, 0, err
		}

		users = append(users, user)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return users, total, nil
}

// FindByID mengambil satu user berdasarkan ID.
func (r *userRepository) FindByID(
	ctx context.Context,
	id int,
) (model.User, error) {

	query := `
		SELECT id, username, email, password, is_active, created_at
		FROM users
		WHERE id = $1
	`

	var user model.User

	err := r.pool.QueryRow(ctx, query, id).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.Password,
		&user.IsActive,
		&user.CreatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.User{}, ErrNotFound
		}

		return model.User{}, err
	}

	return user, nil
}

// Create membuat user baru.
func (r *userRepository) Create(
	ctx context.Context,
	user model.User,
) (model.User, error) {

	query := `
		INSERT INTO users (username, email, password, is_active)
		VALUES ($1, $2, $3, $4)
		RETURNING id, username, email, password, is_active, created_at
	`

	var result model.User

	err := r.pool.QueryRow(
		ctx,
		query,
		user.Username,
		user.Email,
		user.Password,
		user.IsActive,
	).Scan(
		&result.ID,
		&result.Username,
		&result.Email,
		&result.Password,
		&result.IsActive,
		&result.CreatedAt,
	)

	if err != nil {
		var pgErr *pgconn.PgError

		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return model.User{}, ErrDuplicate
		}

		return model.User{}, err
	}

	return result, nil
}

// Update memperbarui data user.
func (r *userRepository) Update(
	ctx context.Context,
	user model.User,
) (model.User, error) {

	query := `
		UPDATE users
		SET username = $1,
			email = $2,
			is_active = $3
		WHERE id = $4
		RETURNING id, username, email, password, is_active, created_at
	`

	var result model.User

	err := r.pool.QueryRow(
		ctx,
		query,
		user.Username,
		user.Email,
		user.IsActive,
		user.ID,
	).Scan(
		&result.ID,
		&result.Username,
		&result.Email,
		&result.Password,
		&result.IsActive,
		&result.CreatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.User{}, ErrNotFound
		}

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return model.User{}, ErrDuplicate
		}

		return model.User{}, err
	}

	return result, nil
}

// Delete menghapus user berdasarkan ID.
func (r *userRepository) Delete(
	ctx context.Context,
	id int,
) error {

	query := `
		DELETE FROM users
		WHERE id = $1
	`

	result, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}
