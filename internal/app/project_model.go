package app

import "time"

type Project struct {
	Archived    bool      `json:"archived"`
	Deleted     bool      `json:"deleted,omitempty"`
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
