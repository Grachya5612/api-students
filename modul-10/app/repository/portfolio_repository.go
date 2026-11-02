package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"api-students/app/model"
)

const MaxProjects = 20

var (
	ErrPortfolioNotFound = errors.New("portofolio tidak ditemukan")
	ErrProjectNotFound   = errors.New("proyek tidak ditemukan")
	ErrProjectLimit      = errors.New("batas proyek sudah tercapai")
	ErrInvalidProjectID  = errors.New("id proyek tidak valid")
)

// Struktur dokumen khusus repository. Hanya di sini ada tipe ObjectID.
type portfolioDoc struct {
	ID           bson.ObjectID    `bson:"_id"`
	StudentID    int              `bson:"student_id"`
	Headline     string           `bson:"headline"`
	Bio          string           `bson:"bio"`
	Projects     []projectDoc     `bson:"projects"`
	Certificates []certificateDoc `bson:"certificates"`
	UpdatedAt    time.Time        `bson:"updated_at"`
}

type projectDoc struct {
	ID           bson.ObjectID `bson:"_id"`
	Title        string        `bson:"title"`
	Technologies []string      `bson:"technologies"`
	URL          string        `bson:"url"`
	Year         int           `bson:"year"`
}

type certificateDoc struct {
	ID       bson.ObjectID `bson:"_id"`
	Name     string        `bson:"name"`
	Issuer   string        `bson:"issuer"`
	IssuedAt time.Time     `bson:"issued_at"`
}

type techStatDoc struct {
	Technology string `bson:"technology"`
	Count      int    `bson:"count"`
}

type PortfolioRepository struct {
	coll *mongo.Collection
}

func NewPortfolioRepository(db *mongo.Database) *PortfolioRepository {
	return &PortfolioRepository{coll: db.Collection("student_portfolios")}
}

// EnsureIndexes membuat index yang dibutuhkan. Aman dipanggil setiap startup.
func (r *PortfolioRepository) EnsureIndexes(ctx context.Context) error {
	_, err := r.coll.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "student_id", Value: 1}},
			Options: options.Index().
				SetUnique(true).
				SetName("uniq_student_id"),
		},
		{
			Keys: bson.D{{Key: "projects.technologies", Value: 1}},
			Options: options.Index().
				SetName("idx_projects_technologies"),
		},
	})
	return err
}

// Upsert membuat portofolio jika belum ada, atau memperbarui headline dan bio.
// Array projects dan certificates tidak ikut tertimpa.
func (r *PortfolioRepository) Upsert(ctx context.Context, studentID int, headline, bio string) error {
	filter := bson.M{"student_id": studentID}
	update := bson.M{
		"$set": bson.M{
			"headline":   headline,
			"bio":        bio,
			"updated_at": time.Now().UTC(),
		},
		"$setOnInsert": bson.M{
			"projects":     bson.A{},
			"certificates": bson.A{},
		},
	}

	opts := options.UpdateOne().SetUpsert(true)
	_, err := r.coll.UpdateOne(ctx, filter, update, opts)
	if mongo.IsDuplicateKeyError(err) {
		// Dua request bersamaan sama-sama insert; yang kalah coba sekali lagi.
		_, err = r.coll.UpdateOne(ctx, filter, update, opts)
	}
	return err
}

func (r *PortfolioRepository) FindByStudentID(ctx context.Context, studentID int) (*model.Portfolio, error) {
	var doc portfolioDoc
	err := r.coll.FindOne(ctx, bson.M{"student_id": studentID}).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrPortfolioNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("membaca portofolio: %w", err)
	}
	return toPortfolio(doc), nil
}

// AddProject menambah satu proyek dengan $push. Batas 20 dijaga atomik:
// filter hanya cocok jika elemen indeks 19 belum ada, yaitu jumlah proyek < 20.
// Dua request bersamaan tidak bisa sama-sama lolos, karena update satu
// dokumen dijalankan atomik oleh MongoDB.
func (r *PortfolioRepository) AddProject(ctx context.Context, studentID int, in model.ProjectInput) (model.Project, error) {
	id := bson.NewObjectID()
	doc := projectDoc{
		ID:           id,
		Title:        in.Title,
		Technologies: in.Technologies,
		URL:          in.URL,
		Year:         in.Year,
	}

	filter := bson.M{
		"student_id":  studentID,
		"projects.19": bson.M{"$exists": false},
	}
	update := bson.M{
		"$push": bson.M{"projects": doc},
		"$set":  bson.M{"updated_at": time.Now().UTC()},
	}

	res, err := r.coll.UpdateOne(ctx, filter, update)
	if err != nil {
		return model.Project{}, fmt.Errorf("menambah proyek: %w", err)
	}
	if res.MatchedCount == 0 {
		return model.Project{}, r.noPortfolioOrLimit(ctx, studentID)
	}

	return model.Project{
		ID:           id.Hex(),
		Title:        in.Title,
		Technologies: in.Technologies,
		URL:          in.URL,
		Year:         in.Year,
	}, nil
}

// DeleteProject menghapus satu proyek dengan $pull.
func (r *PortfolioRepository) DeleteProject(ctx context.Context, studentID int, projectID string) error {
	oid, err := bson.ObjectIDFromHex(projectID)
	if err != nil {
		return ErrInvalidProjectID
	}

	res, err := r.coll.UpdateOne(ctx,
		bson.M{"student_id": studentID, "projects._id": oid},
		bson.M{
			"$pull": bson.M{"projects": bson.M{"_id": oid}},
			"$set":  bson.M{"updated_at": time.Now().UTC()},
		},
	)
	if err != nil {
		return fmt.Errorf("menghapus proyek: %w", err)
	}
	if res.MatchedCount == 0 {
		return ErrProjectNotFound
	}
	return nil
}

// TechnologyStats menghitung kemunculan setiap teknologi di seluruh proyek.
// Dua $unwind: pertama membuka array projects, kedua membuka technologies.
func (r *PortfolioRepository) TechnologyStats(ctx context.Context, limit int) ([]model.TechStat, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$unwind", Value: "$projects"}},
		{{Key: "$unwind", Value: "$projects.technologies"}},
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$projects.technologies"},
			{Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}},
		}}},
		{{Key: "$sort", Value: bson.D{
			{Key: "count", Value: -1},
			{Key: "_id", Value: 1},
		}}},
		{{Key: "$limit", Value: limit}},
		{{Key: "$project", Value: bson.D{
			{Key: "_id", Value: 0},
			{Key: "technology", Value: "$_id"},
			{Key: "count", Value: 1},
		}}},
	}

	cur, err := r.coll.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("menjalankan agregasi teknologi: %w", err)
	}
	defer cur.Close(ctx)

	var docs []techStatDoc
	if err := cur.All(ctx, &docs); err != nil {
		return nil, fmt.Errorf("membaca hasil agregasi: %w", err)
	}

	out := make([]model.TechStat, 0, len(docs))
	for _, d := range docs {
		out = append(out, model.TechStat{Technology: d.Technology, Count: d.Count})
	}
	return out, nil
}

// noPortfolioOrLimit dipanggil saat update tidak cocok. Membedakan dua
// penyebab: portofolio belum ada, atau portofolio ada tapi sudah 20 proyek.
func (r *PortfolioRepository) noPortfolioOrLimit(ctx context.Context, studentID int) error {
	n, err := r.coll.CountDocuments(ctx, bson.M{"student_id": studentID})
	if err != nil {
		return fmt.Errorf("memeriksa portofolio: %w", err)
	}
	if n == 0 {
		return ErrPortfolioNotFound
	}
	return ErrProjectLimit
}

func toPortfolio(d portfolioDoc) *model.Portfolio {
	p := &model.Portfolio{
		StudentID:    d.StudentID,
		Headline:     d.Headline,
		Bio:          d.Bio,
		UpdatedAt:    d.UpdatedAt,
		Projects:     make([]model.Project, 0, len(d.Projects)),
		Certificates: make([]model.Certificate, 0, len(d.Certificates)),
	}
	for _, pr := range d.Projects {
		p.Projects = append(p.Projects, model.Project{
			ID:           pr.ID.Hex(),
			Title:        pr.Title,
			Technologies: pr.Technologies,
			URL:          pr.URL,
			Year:         pr.Year,
		})
	}
	for _, c := range d.Certificates {
		p.Certificates = append(p.Certificates, model.Certificate{
			ID:       c.ID.Hex(),
			Name:     c.Name,
			Issuer:   c.Issuer,
			IssuedAt: c.IssuedAt,
		})
	}
	return p
}
