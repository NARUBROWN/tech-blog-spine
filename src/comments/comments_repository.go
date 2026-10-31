package comments

import (
	"database/sql"
	"github.com/NARUBROWN/tech-blog-spine/internal/store"
)

type CommentsRepository struct{ db *sql.DB }

func NewCommentsRepository(s *store.MySQLStore) *CommentsRepository {
	return &CommentsRepository{db: s.FeedbackDB()}
}
