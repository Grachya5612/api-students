package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"api-students/app/model"
	"api-students/app/repository"
)

var (
	ErrStudentNotFound = errors.New("mahasiswa tidak ditemukan")
	ErrInvalidInput    = errors.New("input tidak valid")
)

const (
	maxTechnologies = 10
	maxTitleLength  = 100
	minProjectYear  = 2000
	maxProjectYear  = 2100
	techStatLimit   = 10
)

type PortfolioService struct {
	portfolios *repository.PortfolioRepository
	students   repository.StudentRepository
}

func NewPortfolioService(
	portfolios *repository.PortfolioRepository,
	students repository.StudentRepository,
) *PortfolioService {
	return &PortfolioService{portfolios: portfolios, students: students}
}

// ensureStudent menolak ID yang tidak ada di PostgreSQL. MongoDB tidak punya
// foreign key, jadi pengecekan ini wajib dilakukan di kode.
func (s *PortfolioService) ensureStudent(ctx context.Context, studentID int) error {
	exists, err := s.students.Exists(ctx, studentID)
	if err != nil {
		return fmt.Errorf("memeriksa mahasiswa: %w", err)
	}
	if !exists {
		return ErrStudentNotFound
	}
	return nil
}

func (s *PortfolioService) Get(ctx context.Context, studentID int) (*model.Portfolio, error) {
	if err := s.ensureStudent(ctx, studentID); err != nil {
		return nil, err
	}
	return s.portfolios.FindByStudentID(ctx, studentID)
}

func (s *PortfolioService) Upsert(ctx context.Context, studentID int, headline, bio string) error {
	if err := s.ensureStudent(ctx, studentID); err != nil {
		return err
	}
	return s.portfolios.Upsert(ctx, studentID, headline, bio)
}

func (s *PortfolioService) AddProject(ctx context.Context, studentID int, in model.ProjectInput) (model.Project, error) {
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" || len(in.Title) > maxTitleLength {
		return model.Project{}, fmt.Errorf("%w: judul wajib diisi, maksimal %d karakter", ErrInvalidInput, maxTitleLength)
	}
	if len(in.Technologies) == 0 || len(in.Technologies) > maxTechnologies {
		return model.Project{}, fmt.Errorf("%w: teknologi harus 1 sampai %d item", ErrInvalidInput, maxTechnologies)
	}
	if in.Year < minProjectYear || in.Year > maxProjectYear {
		return model.Project{}, fmt.Errorf("%w: tahun harus antara %d dan %d", ErrInvalidInput, minProjectYear, maxProjectYear)
	}

	if err := s.ensureStudent(ctx, studentID); err != nil {
		return model.Project{}, err
	}
	return s.portfolios.AddProject(ctx, studentID, in)
}

func (s *PortfolioService) DeleteProject(ctx context.Context, studentID int, projectID string) error {
	if err := s.ensureStudent(ctx, studentID); err != nil {
		return err
	}
	return s.portfolios.DeleteProject(ctx, studentID, projectID)
}

func (s *PortfolioService) TechnologyStats(ctx context.Context) ([]model.TechStat, error) {
	return s.portfolios.TechnologyStats(ctx, techStatLimit)
}
