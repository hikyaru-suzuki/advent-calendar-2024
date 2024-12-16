package main

import (
	"context"
	"database/sql"
	"math/rand"
	"net/http"
	"os"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
	"golang.org/x/crypto/bcrypt"
)

type contextKey struct{}

type Context struct {
	User *User
}

func Extract(ctx context.Context) *Context {
	value := ctx.Value(contextKey{})
	if value == nil {
		return nil
	}
	return value.(*Context)
}

func newHTTPHandler() (http.Handler, error) {
	conn, err := newConnection(
		os.Getenv("DB_HOST"), os.Getenv("DB_PORT"), os.Getenv("DB_USER"),
		os.Getenv("DB_PASS"), os.Getenv("DB_NAME"), os.Getenv("DB_SSL"),
	)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	h := &handler{
		db: &dbExt{conn},
		randUtil: &randUtilImpl{
			Rand: rand.New(rand.NewSource(time.Now().UnixNano())),
		},
		timer: &timerImpl{},
	}

	return setupEcho(h), nil
}

func setupEcho(h *handler) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.Use(otelecho.Middleware("")) // 空にするとリクエストヘッダーからホスト名が自動で設定される
	e.Use(middleware.Recover())
	e.Use(middleware.RequestID())

	e.Use(middleware.BasicAuth(func(email string, password string, e echo.Context) (bool, error) {
		ctx := e.Request().Context()
		if e.Request().URL.Path == "/user" { // ユーザー登録は認証不要
			return true, nil
		}

		row := h.db.QueryRowContext(ctx, "SELECT id, name, email, password_hash, created_at, updated_at FROM users WHERE email = $1", email)
		user := &User{}
		if err := row.Scan(&user.ID, &user.Name, &user.Email, &user.PasswordHash, &user.CreatedAt, &user.UpdatedAt); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return false, nil
			}
			return false, errors.WithStack(err)
		}
		ctx = context.WithValue(ctx, contextKey{}, &Context{
			User: user,
		})
		e.SetRequest(e.Request().WithContext(ctx))

		return !errors.Is(
			bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)),
			bcrypt.ErrMismatchedHashAndPassword,
		), nil
	}))

	e.POST("/user", h.handlePostUser)
	e.GET("/articles", h.handleGetArticleList)
	e.GET("/article/:article_id", h.handleGetArticle)
	e.POST("/article", h.handlePostArticle)
	e.PATCH("/article/:article_id", h.handlePatchArticle)
	e.DELETE("/article/:article_id", h.handleDeleteArticle)
	e.GET("/favorite/articles", h.handleGetFavoriteArticleList)
	e.POST("/favorite/article/:article_id", h.handlePostFavoriteArticle)

	return e
}
