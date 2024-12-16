package main

import (
	"context"
	"database/sql"
	"math/rand"
	"net/http"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"
)

type randUtil interface {
	NewString() string
	Hit(rate, denominator int) bool
}

type randUtilImpl struct {
	*rand.Rand
}

func (r *randUtilImpl) NewString() string {
	return uuid.NewString()
}

func (r *randUtilImpl) Hit(rate, denominator int) bool {
	return rate > r.Intn(denominator)
}

type timer interface {
	Now() time.Time
}

type timerImpl struct{}

func (t *timerImpl) Now() time.Time {
	return time.Now()
}

type handler struct {
	db       *dbExt
	randUtil randUtil
	timer    timer
}

func (h *handler) handlePostUser(c echo.Context) error {
	ctx := c.Request().Context()

	type request struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	req := &request{}
	if err := c.Bind(req); err != nil {
		return errors.WithStack(err)
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return errors.WithStack(err)
	}
	user := &User{
		ID:           h.randUtil.NewString(),
		Name:         req.Name,
		Email:        req.Email,
		PasswordHash: string(passwordHash),
		CreatedAt:    h.timer.Now(),
		UpdatedAt:    h.timer.Now(),
	}

	if err := h.db.Transaction(ctx, func(ctx context.Context, tx *txExt) error {
		stmt, err := tx.PrepareContext(ctx, "INSERT INTO users (id, name, email, password_hash, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6)")
		if err != nil {
			return errors.WithStack(err)
		}

		if _, err := stmt.ExecContext(ctx, user.ID, user.Name, user.Email, user.PasswordHash, user.CreatedAt, user.UpdatedAt); err != nil {
			return errors.WithStack(err)
		}
		return nil
	}); err != nil {
		return errors.WithStack(err)
	}

	return c.NoContent(http.StatusOK)
}

func (h *handler) handleGetArticleList(c echo.Context) error {
	ctx := c.Request().Context()

	rows, err := h.db.QueryContext(ctx, "SELECT id, title, body, user_id, total_favorite_count, created_at, updated_at FROM articles ORDER BY created_at DESC LIMIT 100")
	if err != nil {
		return errors.WithStack(err)
	}
	defer rows.Close()

	type responseItem struct {
		ArticleID string `json:"article_id"`
		Title     string `json:"title"`
	}
	type response struct {
		List []*responseItem `json:"list"`
	}
	res := &response{
		List: make([]*responseItem, 0),
	}
	for rows.Next() {
		article := &Article{}
		if err := rows.Scan(&article.ID, &article.Title, &article.Body, &article.UserID, &article.TotalFavoriteCount, &article.CreatedAt, &article.UpdatedAt); err != nil {
			return errors.WithStack(err)
		}
		res.List = append(res.List, &responseItem{
			ArticleID: article.ID,
			Title:     article.Title,
		})
	}

	return c.JSON(http.StatusOK, res)
}

func (h *handler) handleGetArticle(c echo.Context) error {
	ctx := c.Request().Context()

	type request struct {
		ArticleID string `param:"article_id"`
	}
	req := &request{}
	if err := c.Bind(req); err != nil {
		return errors.WithStack(err)
	}

	row := h.db.QueryRowContext(ctx, "SELECT id, title, body, user_id, total_favorite_count, created_at, updated_at FROM articles WHERE id = $1", req.ArticleID)
	article := &Article{}
	if err := row.Scan(&article.ID, &article.Title, &article.Body, &article.UserID, &article.TotalFavoriteCount, &article.CreatedAt, &article.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "Not Found")
		}
		return errors.WithStack(err)
	}

	return c.JSON(http.StatusOK, article)
}

func (h *handler) handlePostArticle(c echo.Context) error {
	ctx := c.Request().Context()
	userID := Extract(ctx).User.ID

	type request struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	req := &request{}
	if err := c.Bind(req); err != nil {
		return errors.WithStack(err)
	}

	articleID := h.randUtil.NewString()
	article := &Article{
		ID:                 articleID,
		Title:              req.Title,
		Body:               req.Body,
		UserID:             userID,
		TotalFavoriteCount: 0,
		CreatedAt:          h.timer.Now(),
		UpdatedAt:          h.timer.Now(),
	}

	if err := h.db.Transaction(ctx, func(ctx context.Context, tx *txExt) error {
		stmt, err := tx.PrepareContext(ctx, "INSERT INTO articles (id, title, body, user_id, total_favorite_count, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7)")
		if err != nil {
			return errors.WithStack(err)
		}
		defer stmt.Close()
		if _, err := stmt.ExecContext(ctx, article.ID, article.Title, article.Body, article.UserID, article.TotalFavoriteCount, article.CreatedAt, article.UpdatedAt); err != nil {
			return errors.WithStack(err)
		}
		return nil
	}); err != nil {
		return errors.WithStack(err)
	}

	type response struct {
		ArticleID string `json:"article_id"`
	}
	return c.JSON(http.StatusOK, &response{
		ArticleID: articleID,
	})
}

func (h *handler) handlePatchArticle(c echo.Context) error {
	ctx := c.Request().Context()
	userID := Extract(ctx).User.ID

	type request struct {
		ArticleID string `param:"article_id"`
		Title     string `json:"title"`
		Body      string `json:"body"`
	}
	req := &request{}
	if err := c.Bind(req); err != nil {
		return errors.WithStack(err)
	}

	if err := h.db.Transaction(ctx, func(ctx context.Context, tx *txExt) error {
		article := &Article{}

		stmt, err := tx.PrepareContext(ctx, "SELECT id, title, body, user_id, total_favorite_count, created_at, updated_at FROM articles WHERE id = $1")
		if err != nil {
			return errors.WithStack(err)
		}

		row := stmt.QueryRowContext(ctx, req.ArticleID)
		if err := row.Scan(&article.ID, &article.Title, &article.Body, &article.UserID, &article.TotalFavoriteCount, &article.CreatedAt, &article.UpdatedAt); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return echo.NewHTTPError(http.StatusNotFound, "Not Found")
			}
			return errors.WithStack(err)
		}
		if article.UserID != userID {
			return echo.NewHTTPError(http.StatusForbidden, "Forbidden")
		}
		if req.Title != "" {
			article.Title = req.Title
		}
		if req.Body != "" {
			article.Body = req.Body
		}
		article.UpdatedAt = h.timer.Now()
		stmt, err = tx.PrepareContext(ctx, "UPDATE articles SET title = $1, body = $2, updated_at = $3 WHERE id = $4")
		if err != nil {
			return errors.WithStack(err)
		}
		defer stmt.Close()
		if _, err := stmt.ExecContext(ctx, article.Title, article.Body, article.UpdatedAt, article.ID); err != nil {
			return errors.WithStack(err)
		}
		return nil
	}); err != nil {
		return errors.WithStack(err)
	}

	return c.NoContent(http.StatusOK)
}

func (h *handler) handleDeleteArticle(c echo.Context) error {
	ctx := c.Request().Context()
	userID := Extract(ctx).User.ID

	type request struct {
		ArticleID string `param:"article_id"`
	}
	req := &request{}
	if err := c.Bind(req); err != nil {
		return errors.WithStack(err)
	}

	if err := h.db.Transaction(ctx, func(ctx context.Context, tx *txExt) error {
		article := &Article{}
		row := tx.QueryRowContext(ctx, "SELECT id, title, body, user_id, total_favorite_count, created_at, updated_at FROM articles WHERE id = $1", req.ArticleID)
		if err := row.Scan(&article.ID, &article.Title, &article.Body, &article.UserID, &article.TotalFavoriteCount, &article.CreatedAt, &article.UpdatedAt); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return echo.NewHTTPError(http.StatusNotFound, "Not Found")
			}
			return errors.WithStack(err)
		}
		if article.UserID != userID {
			return echo.NewHTTPError(http.StatusForbidden, "Forbidden")
		}
		stmt, err := tx.PrepareContext(ctx, "DELETE FROM articles WHERE id = $1")
		if err != nil {
			return errors.WithStack(err)
		}
		defer stmt.Close()
		if _, err := stmt.ExecContext(ctx, article.ID); err != nil {
			return errors.WithStack(err)
		}
		return nil
	}); err != nil {
		return errors.WithStack(err)
	}

	return c.NoContent(http.StatusOK)
}

func (h *handler) handleGetFavoriteArticleList(c echo.Context) error {
	ctx := c.Request().Context()
	userID := Extract(ctx).User.ID

	rows, err := h.db.QueryContext(ctx, "SELECT id, title, body, user_id, total_favorite_count, created_at, updated_at FROM articles WHERE id IN (SELECT article_id FROM users_articles WHERE user_id = $1) ORDER BY created_at DESC", userID)
	if err != nil {
		return errors.WithStack(err)
	}
	defer rows.Close()
	articleList := make([]*Article, 0)
	for rows.Next() {
		article := &Article{}
		if err := rows.Scan(&article.ID, &article.Title, &article.Body, &article.UserID, &article.TotalFavoriteCount, &article.CreatedAt, &article.UpdatedAt); err != nil {
			return errors.WithStack(err)
		}
		articleList = append(articleList, article)
	}

	type responseItem struct {
		ArticleID string `json:"article_id"`
		Title     string `json:"title"`
	}
	type response struct {
		List []*responseItem `json:"list"`
	}
	res := &response{
		List: make([]*responseItem, 0),
	}
	for _, article := range articleList {
		res.List = append(res.List, &responseItem{
			ArticleID: article.ID,
			Title:     article.Title,
		})
	}
	return c.JSON(http.StatusOK, res)
}

func (h *handler) handlePostFavoriteArticle(c echo.Context) error {
	ctx := c.Request().Context()
	userID := Extract(ctx).User.ID

	type request struct {
		ArticleID string `param:"article_id"`
	}
	req := &request{}
	if err := c.Bind(req); err != nil {
		return errors.WithStack(err)
	}

	if err := h.db.Transaction(ctx, func(ctx context.Context, tx *txExt) error {
		row := tx.QueryRowContext(ctx, "SELECT id, title, body, user_id, total_favorite_count, created_at, updated_at FROM articles WHERE id = $1", req.ArticleID)
		article := &Article{}
		if err := row.Scan(&article.ID, &article.Title, &article.Body, &article.UserID, &article.TotalFavoriteCount, &article.CreatedAt, &article.UpdatedAt); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return echo.NewHTTPError(http.StatusNotFound, "Not Found")
			}
			return errors.WithStack(err)
		}

		stmt, err := tx.PrepareContext(ctx, "UPDATE articles SET total_favorite_count = $1, updated_at = $2 WHERE id = $3")
		if err != nil {
			return errors.WithStack(err)
		}
		defer stmt.Close()
		if _, err := stmt.ExecContext(ctx, article.TotalFavoriteCount+1, h.timer.Now(), article.ID); err != nil {
			return errors.WithStack(err)
		}

		stmt, err = tx.PrepareContext(ctx, "INSERT INTO users_articles (user_id, article_id, created_at, updated_at) VALUES ($1, $2, $3, $4)")
		if err != nil {
			return errors.WithStack(err)
		}
		defer stmt.Close()
		if _, err := stmt.ExecContext(ctx, userID, req.ArticleID, h.timer.Now(), h.timer.Now()); err != nil {
			return errors.WithStack(err)
		}
		return nil
	}); err != nil {
		return errors.WithStack(err)
	}

	return c.NoContent(http.StatusOK)
}
