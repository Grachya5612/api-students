package model

import "time"

type Portfolio struct {
	StudentID    int           `json:"student_id"`
	Headline     string        `json:"headline"`
	Bio          string        `json:"bio"`
	Projects     []Project     `json:"projects"`
	Certificates []Certificate `json:"certificates"`
	UpdatedAt    time.Time     `json:"updated_at"`
}

type Project struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Technologies []string `json:"technologies"`
	URL          string   `json:"url"`
	Year         int      `json:"year"`
}

// ProjectInput dipakai sebagai body request sekaligus parameter service.
type ProjectInput struct {
	Title        string   `json:"title"`
	Technologies []string `json:"technologies"`
	URL          string   `json:"url"`
	Year         int      `json:"year"`
}

type Certificate struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Issuer   string    `json:"issuer"`
	IssuedAt time.Time `json:"issued_at"`
}

type TechStat struct {
	Technology string `json:"technology"`
	Count      int    `json:"count"`
}
