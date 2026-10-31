package analytics

import (
	"database/sql"
	"github.com/NARUBROWN/tech-blog-spine/internal/store"
)

type AnalyticsRepository struct{ db *sql.DB }

func NewAnalyticsRepository(s *store.MySQLStore) *AnalyticsRepository {
	return &AnalyticsRepository{s.FeedbackDB()}
}
