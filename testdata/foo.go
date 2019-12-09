package foos

import (
	"time"
)

type Post struct {
	ID        int64  `db:"id,pk"`
	UUID      string `db:"uuid,immutable"`
	AccountID int64  `db:"account_id,immutable"`

	CreatedAt time.Time `db:"created_at,immutable"`
	UpdatedAt time.Time `db:"updated_at"`
	DeletedAt time.Time `db:"deleted_at,nullable"`

	State      PostState `db:"state,#state"`
	PublisedAt time.Time `db:"published_at,nullable,#state"`

	Title string  `db:"title,#content"`
	Body  string  `db:"description,#content"`
	Music *string `db:"music,#content"`
}

type PostState string

const (
	PostStateDraft     PostState = "draft"
	PostStatePublished PostState = "published"
)
