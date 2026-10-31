package store

import (
	"context"
	"errors"

	"github.com/NARUBROWN/tech-blog-spine/internal/model"
)

var (
	ErrNotFound = errors.New("resource not found")
	ErrConflict = errors.New("resource conflict")
)

type CreatePostParams struct {
	Request    model.PostRequest
	CategoryID int64
	Username   string
	Slug       string
}

type UpdatePostParams struct {
	PostID     int64
	CategoryID int64
	Request    model.PostRequest
	Slug       string
}

type Store interface {
	CreateUser(context.Context, model.User) error
	UserByID(context.Context, int64) (model.User, error)
	UserByUsername(context.Context, string) (model.User, error)
	UpdateUser(context.Context, model.User) (model.User, error)
	CreateCategory(context.Context, string) error
	ListCategories(context.Context) ([]model.Category, error)
	CreatePost(context.Context, CreatePostParams) error
	UpdatePost(context.Context, UpdatePostParams) (string, error)
	PostByID(context.Context, int64) (model.Post, error)
	PostBySlug(context.Context, string) (model.Post, error)
	ListPosts(context.Context, string, int, int) ([]model.Post, int64, error)
	SearchPosts(context.Context, string, int, int) ([]model.Post, int64, error)
	DeletePost(context.Context, int64) error
	IncrementViewCount(context.Context, int64) error
	AddLike(context.Context, int64, int64) error
	RemoveLike(context.Context, int64, int64) error
	UsersWhoLiked(context.Context, int64) ([]model.User, error)
	SaveImageMetadata(context.Context, model.ImageMetadata) (model.ImageMetadata, error)
	ImageByPublicID(context.Context, string) (model.ImageMetadata, error)
}
