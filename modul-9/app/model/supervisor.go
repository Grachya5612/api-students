package model

type Supervisor struct {
	ID              int    `json:"id"`
	Name            string `json:"name"`
	Quota           int    `json:"quota"`
	SupervisedCount int    `json:"supervised_count"`
	Version         int    `json:"version"`
}

// AssignRequest adalah body untuk POST /supervisors/:id/assign.
type AssignRequest struct {
	StudentID int `json:"student_id" validate:"required,min=1"`
}

// SwitchRequest adalah body untuk POST /supervisions/switch.
type SwitchRequest struct {
	StudentID      int `json:"student_id" validate:"required,min=1"`
	ToSupervisorID int `json:"to_supervisor_id" validate:"required,min=1"`
}