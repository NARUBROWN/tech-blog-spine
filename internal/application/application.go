package application

import (
	"os"

	"github.com/NARUBROWN/spine"
	"github.com/NARUBROWN/spine/interceptor/cors"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/httpx"
	"github.com/NARUBROWN/tech-blog-spine/internal/blog"
	"github.com/NARUBROWN/tech-blog-spine/internal/config"
	"github.com/NARUBROWN/tech-blog-spine/internal/security"
	"github.com/NARUBROWN/tech-blog-spine/internal/store"
	auth "github.com/NARUBROWN/tech-blog-spine/src/auth"
	categories "github.com/NARUBROWN/tech-blog-spine/src/categories"
	images "github.com/NARUBROWN/tech-blog-spine/src/images"
	likes "github.com/NARUBROWN/tech-blog-spine/src/likes"
	posts "github.com/NARUBROWN/tech-blog-spine/src/posts"
	users "github.com/NARUBROWN/tech-blog-spine/src/users"
)

type HealthController struct{}

func NewHealthController() *HealthController { return &HealthController{} }

func (c *HealthController) Get() httpx.Response[map[string]string] {
	return httpx.Response[map[string]string]{Body: map[string]string{"status": "ok"}}
}

func NewConfig() config.Config {
	loaded, err := config.Load()
	if err != nil {
		panic(err)
	}
	return loaded
}

func NewCORSInterceptor(config config.Config) *cors.CORSInterceptor {
	interceptor, err := cors.NewValidated(corsConfig(config))
	if err != nil {
		panic(err)
	}
	return interceptor
}

func corsConfig(config config.Config) cors.Config {
	return cors.Config{
		AllowOrigins:     config.AllowedOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
		AllowHeaders:     []string{"Content-Type", "Authorization"},
		AllowCredentials: true,
	}
}

func ProvideMySQLStore(config config.Config) *store.MySQLStore {
	return store.NewMySQLStore(config)
}

func ProvideRedisRefreshTokenStore(config config.Config) *security.RedisRefreshTokenStore {
	return security.NewRedisRefreshTokenStore(config)
}

func ProvideTokenManager(config config.Config) *security.TokenManager {
	return security.NewTokenManager(config)
}

func ProvideImageStorage(config config.Config) *blog.ImageStorage {
	storage, err := blog.NewImageStorage(config)
	if err != nil {
		panic(err)
	}
	return storage
}

func ProvideBlogService(mysql *store.MySQLStore, tokenStore *security.RedisRefreshTokenStore, tokens *security.TokenManager, images *blog.ImageStorage) *blog.Service {
	return blog.NewService(mysql, tokenStore, tokens, images)
}

func ProvideAuthInterceptor(tokens *security.TokenManager, tokenStore *security.RedisRefreshTokenStore) *security.AuthInterceptor {
	return security.NewAuthInterceptor(tokens, tokenStore)
}

// BuildApplication은 서버 실행, 계약 검사와 오프라인 검증이 함께 사용하는 composition root입니다.
func BuildApplication() (spine.App, boot.Options, error) {
	loadedConfig, err := config.Load()
	if err != nil {
		return nil, boot.Options{}, err
	}
	if _, err := cors.NewValidated(corsConfig(loadedConfig)); err != nil {
		return nil, boot.Options{}, err
	}
	if _, err := blog.NewImageStorage(loadedConfig); err != nil {
		return nil, boot.Options{}, err
	}
	app := spine.New()
	app.Constructor(
		NewConfig,
		ProvideMySQLStore,
		ProvideRedisRefreshTokenStore,
		ProvideTokenManager,
		ProvideImageStorage,
		ProvideBlogService,
		ProvideAuthInterceptor,
		NewCORSInterceptor,
	)
	app.InterceptorFor(boot.InterceptorHTTP, (*cors.CORSInterceptor)(nil))
	app.InterceptorFor(boot.InterceptorHTTP, (*security.AuthInterceptor)(nil))
	if err := images.Register(app); err != nil {
		return nil, boot.Options{}, err
	}
	if err := likes.Register(app); err != nil {
		return nil, boot.Options{}, err
	}
	if err := categories.Register(app); err != nil {
		return nil, boot.Options{}, err
	}
	if err := posts.Register(app); err != nil {
		return nil, boot.Options{}, err
	}
	if err := users.Register(app); err != nil {
		return nil, boot.Options{}, err
	}
	if err := auth.Register(app); err != nil {
		return nil, boot.Options{}, err
	}
	app.Constructor(NewHealthController)
	app.Route("GET", "/health", (*HealthController).Get)

	address := ":8080"
	if port := os.Getenv("SPINE_PORT"); port != "" {
		address = ":" + port
	}
	return app, boot.Options{
		Address:                address,
		EnableGracefulShutdown: true,
		HTTP:                   &boot.HTTPOptions{MaxBodyBytes: 21 << 20},
	}, nil
}
