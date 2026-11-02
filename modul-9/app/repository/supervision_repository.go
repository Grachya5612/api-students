package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"api-students/app/model"
)

var (
	ErrSupervisorFull  = errors.New("kuota pembimbing sudah penuh")
	ErrAlreadyAssigned = errors.New("mahasiswa sudah memiliki pembimbing")
	ErrNotAssigned     = errors.New("mahasiswa belum memiliki pembimbing")
	ErrSameSupervisor  = errors.New("pembimbing baru sama dengan pembimbing lama")
)

const supervisorColumns = `id, name, quota, supervised_count, version`

type SupervisionRepository interface {
	WithTx(tx DBTX) SupervisionRepository

	FindSupervisorByID(ctx context.Context, id int) (model.Supervisor, error)
	LockStudent(ctx context.Context, studentID int) error
	LockSupervisors(ctx context.Context, ids ...int) error
	FindSupervisorOf(ctx context.Context, studentID int) (int, error)
	InsertSupervision(ctx context.Context, studentID, supervisorID int) error
	DeleteSupervision(ctx context.Context, studentID int) error
	IncrementSupervised(ctx context.Context, supervisorID int) (model.Supervisor, error)
	DecrementSupervised(ctx context.Context, supervisorID int) (model.Supervisor, error)
	UpdateSupervisorQuota(ctx context.Context, id, quota, version int) (model.Supervisor, error)
}

type supervisionPostgresRepository struct {
	db DBTX
}

func NewSupervisionRepository(db DBTX) SupervisionRepository {
	return &supervisionPostgresRepository{db: db}
}

func (r *supervisionPostgresRepository) WithTx(tx DBTX) SupervisionRepository {
	return &supervisionPostgresRepository{db: tx}
}

func scanSupervisor(row pgx.Row) (model.Supervisor, error) {
	var s model.Supervisor
	err := row.Scan(&s.ID, &s.Name, &s.Quota, &s.SupervisedCount, &s.Version)
	return s, err
}

func (r *supervisionPostgresRepository) FindSupervisorByID(ctx context.Context, id int) (model.Supervisor, error) {
	s, err := scanSupervisor(r.db.QueryRow(ctx,
		"SELECT "+supervisorColumns+" FROM supervisors WHERE id = $1", id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Supervisor{}, ErrNotFound
		}
		return model.Supervisor{}, fmt.Errorf("mengambil pembimbing: %w", err)
	}
	return s, nil
}

// LockStudent mengunci baris mahasiswa sampai transaction selesai.
// Semua perubahan pembimbing untuk satu mahasiswa harus lewat kunci ini,
// sehingga dua request untuk mahasiswa yang sama antre satu per satu.
func (r *supervisionPostgresRepository) LockStudent(ctx context.Context, studentID int) error {
	var id int
	err := r.db.QueryRow(ctx,
		"SELECT id FROM students WHERE id = $1 FOR UPDATE", studentID,
	).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("mengunci data mahasiswa: %w", err)
	}
	return nil
}

// LockSupervisors mengunci beberapa pembimbing dalam urutan id menaik.
// Urutan yang seragam mencegah deadlock antar switch yang melibatkan
// pasangan pembimbing yang sama tetapi arahnya berlawanan.
func (r *supervisionPostgresRepository) LockSupervisors(ctx context.Context, ids ...int) error {
	rows, err := r.db.Query(ctx,
		"SELECT id FROM supervisors WHERE id = ANY($1) ORDER BY id FOR UPDATE", ids)
	if err != nil {
		return fmt.Errorf("mengunci pembimbing: %w", err)
	}
	defer rows.Close()

	found := 0
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("membaca pembimbing terkunci: %w", err)
		}
		found++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("mengunci pembimbing: %w", err)
	}
	if found != len(ids) {
		return ErrNotFound
	}
	return nil
}

// FindSupervisorOf mengembalikan id pembimbing mahasiswa, atau ErrNotAssigned.
func (r *supervisionPostgresRepository) FindSupervisorOf(ctx context.Context, studentID int) (int, error) {
	var supervisorID int
	err := r.db.QueryRow(ctx,
		"SELECT supervisor_id FROM supervisions WHERE student_id = $1", studentID,
	).Scan(&supervisorID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrNotAssigned
		}
		return 0, fmt.Errorf("mengambil pembimbing mahasiswa: %w", err)
	}
	return supervisorID, nil
}

func (r *supervisionPostgresRepository) InsertSupervision(ctx context.Context, studentID, supervisorID int) error {
	_, err := r.db.Exec(ctx,
		"INSERT INTO supervisions (student_id, supervisor_id) VALUES ($1, $2)",
		studentID, supervisorID,
	)
	if err != nil {
		// Primary key student_id: pelanggaran berarti mahasiswa sudah punya pembimbing.
		if pgErrorCode(err) == codeUniqueViolation {
			return ErrAlreadyAssigned
		}
		return fmt.Errorf("mencatat penetapan pembimbing: %w", err)
	}
	return nil
}

// DeleteSupervision menghapus penetapan lama. DELETE yang tidak menghapus
// baris tetap sukses di PostgreSQL, jadi jumlah baris diperiksa sendiri.
func (r *supervisionPostgresRepository) DeleteSupervision(ctx context.Context, studentID int) error {
	tag, err := r.db.Exec(ctx,
		"DELETE FROM supervisions WHERE student_id = $1", studentID)
	if err != nil {
		return fmt.Errorf("menghapus penetapan pembimbing: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotAssigned
	}
	return nil
}

// IncrementSupervised mengambil satu slot secara atomik. Version ikut naik,
// sehingga form ubah kuota milik admin mendeteksi perubahan ini.
func (r *supervisionPostgresRepository) IncrementSupervised(ctx context.Context, supervisorID int) (model.Supervisor, error) {
	s, err := scanSupervisor(r.db.QueryRow(ctx,
		`UPDATE supervisors
		 SET supervised_count = supervised_count + 1, version = version + 1
		 WHERE id = $1 AND supervised_count < quota
		 RETURNING `+supervisorColumns, supervisorID))
	if err == nil {
		return s, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return model.Supervisor{}, fmt.Errorf("menambah mahasiswa bimbingan: %w", err)
	}
	if _, findErr := r.FindSupervisorByID(ctx, supervisorID); findErr != nil {
		return model.Supervisor{}, findErr
	}
	return model.Supervisor{}, ErrSupervisorFull
}

func (r *supervisionPostgresRepository) DecrementSupervised(ctx context.Context, supervisorID int) (model.Supervisor, error) {
	s, err := scanSupervisor(r.db.QueryRow(ctx,
		`UPDATE supervisors
		 SET supervised_count = supervised_count - 1, version = version + 1
		 WHERE id = $1
		 RETURNING `+supervisorColumns, supervisorID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Supervisor{}, ErrNotFound
		}
		return model.Supervisor{}, fmt.Errorf("mengurangi mahasiswa bimbingan: %w", err)
	}
	return s, nil
}

// UpdateSupervisorQuota menerapkan optimistic locking, sama seperti
// UpdateQuota pada kelas: hanya berhasil bila version masih sama.
func (r *supervisionPostgresRepository) UpdateSupervisorQuota(ctx context.Context, id, quota, version int) (model.Supervisor, error) {
	s, err := scanSupervisor(r.db.QueryRow(ctx,
		`UPDATE supervisors SET quota = $1, version = version + 1
		 WHERE id = $2 AND version = $3 AND $1 >= supervised_count
		 RETURNING `+supervisorColumns, quota, id, version))
	if err == nil {
		return s, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return model.Supervisor{}, fmt.Errorf("mengubah kuota pembimbing: %w", err)
	}

	current, findErr := r.FindSupervisorByID(ctx, id)
	if findErr != nil {
		return model.Supervisor{}, findErr
	}
	if current.Version != version {
		return model.Supervisor{}, ErrVersionConflict
	}
	return model.Supervisor{}, ErrQuotaBelowFilled
}
