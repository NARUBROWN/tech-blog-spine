package model

import "time"

type Role string

const (
	RoleAuthor Role = "ROLE_AUTHOR"
	RoleAdmin  Role = "ROLE_ADMIN"
	RoleUser   Role = "ROLE_USER"
)

type User struct {
	ID              int64
	Username        string
	Email           string
	HashedPassword  string
	ProfileImageURL string
	Role            Role
	Bio             string
}

type Category struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type Post struct {
	ID             int64
	Title          string
	Slug           string
	Content        string
	PublishedAt    time.Time
	LikeCount      int64
	ViewCount      int64
	ThumbnailURL   string
	SEOTitle       string
	SEODescription string
	SEOKeywords    string
	Author         User
	Category       Category
	Tags           []string
}

type ImageMetadata struct {
	ID           int64
	PublicID     string
	StoredName   string
	OriginalName string
	ContentType  string
	Size         int64
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type UserRequest struct {
	Username         string `json:"username"`
	Email            string `json:"email"`
	ProfileImageURL  string `json:"profileImageUrl"`
	Bio              string `json:"bio"`
	OriginalPassword string `json:"originalPassword"`
}

type TagRequest struct {
	TagNames []string `json:"tagNames"`
}

type PostRequest struct {
	Title          string      `json:"title"`
	Content        string      `json:"content"`
	ThumbnailURL   string      `json:"thumbnailUrl"`
	SEOTitle       string      `json:"seoTitle"`
	SEODescription string      `json:"seoDescription"`
	SEOKeywords    string      `json:"seoKeywords"`
	Tags           *TagRequest `json:"tags"`
}

type LoginResponse struct {
	ID              int64  `json:"id"`
	Username        string `json:"username"`
	Email           string `json:"email"`
	ProfileImageURL string `json:"profileImageUrl"`
	Role            Role   `json:"role"`
}

type UserResponse struct {
	ID              int64  `json:"id"`
	Username        string `json:"username"`
	Email           string `json:"email"`
	ProfileImageURL string `json:"profileImageUrl"`
	Bio             string `json:"bio"`
	Role            Role   `json:"role,omitempty"`
}

type PostResponse struct {
	ID             int64        `json:"id"`
	Title          string       `json:"title"`
	Content        string       `json:"content"`
	PublishedAt    string       `json:"publishedAt"`
	LikeCount      int64        `json:"likeCount"`
	ViewCount      int64        `json:"viewCount"`
	ThumbnailURL   string       `json:"thumbnailUrl"`
	SEOTitle       string       `json:"seoTitle"`
	SEODescription string       `json:"seoDescription"`
	SEOKeywords    string       `json:"seoKeywords"`
	Author         UserResponse `json:"author"`
	Tags           TagResponse  `json:"tags"`
	Category       Category     `json:"category"`
}

type TagResponse struct {
	TagNames []string `json:"tagNames"`
}

type PostEditResponse struct {
	Slug string `json:"slug"`
}

type ImageUploadResponse struct {
	SavedID int64  `json:"savedId"`
	URL     string `json:"url"`
}

type PageResponse[T any] struct {
	Content       []T   `json:"content"`
	Number        int   `json:"number"`
	Size          int   `json:"size"`
	TotalElements int64 `json:"totalElements"`
	TotalPages    int   `json:"totalPages"`
	First         bool  `json:"first"`
	Last          bool  `json:"last"`
	Empty         bool  `json:"empty"`
}

func UserView(user User) UserResponse {
	return UserResponse{
		ID: user.ID, Username: user.Username, Email: user.Email,
		ProfileImageURL: user.ProfileImageURL, Bio: user.Bio,
	}
}

func PostView(post Post) PostResponse {
	return PostResponse{
		ID: post.ID, Title: post.Title, Content: post.Content,
		PublishedAt: post.PublishedAt.Format("2006-01-02T15:04:05"), LikeCount: post.LikeCount,
		ViewCount: post.ViewCount, ThumbnailURL: post.ThumbnailURL,
		SEOTitle: post.SEOTitle, SEODescription: post.SEODescription,
		SEOKeywords: post.SEOKeywords, Author: UserView(post.Author),
		Tags:     TagResponse{TagNames: append([]string(nil), post.Tags...)},
		Category: post.Category,
	}
}
