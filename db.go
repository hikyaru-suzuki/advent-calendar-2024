package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/cockroachdb/errors"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func newConnection(host, port, user, pass, name, sslMode string) (*sql.DB, error) {
	if port == "" {
		port = "5432"
	}
	if name == "" {
		name = "''"
	}
	if sslMode == "" {
		sslMode = "require"
	}
	conn, err := sql.Open("postgres", fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		host, port, user, pass, name, sslMode,
	))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return conn, nil
}

func toAttributes(attrMap map[string]any) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, len(attrMap))
	for k, i := range attrMap {
		switch v := i.(type) {
		case bool:
			attrs = append(attrs, attribute.Bool("blog."+k, v))
		case int:
			attrs = append(attrs, attribute.Int("blog."+k, v))
		case int32:
			attrs = append(attrs, attribute.Int64("blog."+k, int64(v)))
		case int64:
			attrs = append(attrs, attribute.Int64("blog."+k, v))
		case float32:
			attrs = append(attrs, attribute.Float64("blog."+k, float64(v)))
		case float64:
			attrs = append(attrs, attribute.Float64("blog."+k, v))
		case string:
			attrs = append(attrs, attribute.String("blog."+k, v))
		case fmt.Stringer:
			attrs = append(attrs, attribute.String("blog."+k, v.String()))
		}
	}
	return attrs
}

type dbExt struct {
	db *sql.DB
}

func (e *dbExt) Transaction(ctx context.Context, f func(ctx context.Context, tx *txExt) error) (err error) {
	ctx, span1 := tracer.Start(ctx, "Transaction")

	b := backoff.NewExponentialBackOff()
	b.InitialInterval = 2 * time.Second
	b.RandomizationFactor = 0.5
	b.Multiplier = 2
	b.MaxElapsedTime = 60 * time.Second
	execCount := 0

	defer func() {
		span1.SetAttributes(toAttributes(map[string]any{
			"exec_count": execCount,
		})...)
		span1.End()
	}()

	if err := backoff.Retry(func() error {
		execCount++

		var tx *sql.Tx
		ctx, span2 := tracer.Start(ctx, "BeginTx")
		tx, err = e.db.BeginTx(ctx, nil)
		span2.End()
		if err != nil {
			return errors.WithStack(err)
		}

		defer func() {
			ctx = trace.ContextWithSpan(ctx, span1)

			if p := recover(); p != nil {
				_, span3 := tracer.Start(ctx, "Rollback")
				defer span3.End()
				e := tx.Rollback()
				if e != nil {
					log.Printf("ロールバックに失敗しました。: %+v\n", e)
				}
				panic(p)
			}

			if err != nil {
				_, span3 := tracer.Start(ctx, "Rollback")
				defer span3.End()
				e := tx.Rollback()
				if e != nil {
					log.Printf("ロールバックに失敗しました。: %+v\n", e)
				}
				return
			}

			// 正常
			_, span3 := tracer.Start(ctx, "Commit")
			defer span3.End()
			if e := tx.Commit(); e != nil {
				err = errors.WithStack(e)
			}
		}()

		ctx = trace.ContextWithSpan(ctx, span1)
		ctx, span4 := tracer.Start(ctx, "Callback")
		defer span4.End()
		if err = f(ctx, &txExt{tx}); err != nil {
			return errors.WithStack(err)
		}

		return nil
	}, b); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func (e *dbExt) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	ctx, span := tracer.Start(ctx, "QueryContext")
	defer span.End()
	m := make(map[string]any)
	for i, arg := range args {
		m["arg"+strconv.FormatInt(int64(i), 10)] = arg
	}
	m["query"] = query
	span.SetAttributes(toAttributes(m)...)

	rows, err := e.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return rows, nil
}

func (e *dbExt) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	ctx, span := tracer.Start(ctx, "QueryRowContext")
	defer span.End()
	m := make(map[string]any)
	for i, arg := range args {
		m["arg"+strconv.FormatInt(int64(i), 10)] = arg
	}
	m["query"] = query
	span.SetAttributes(toAttributes(m)...)

	row := e.db.QueryRowContext(ctx, query, args...)
	return row
}

type txExt struct {
	tx *sql.Tx
}

func (e *txExt) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	ctx, span := tracer.Start(ctx, "QueryRowContext")
	defer span.End()
	m := make(map[string]any)
	for i, arg := range args {
		m["arg"+strconv.FormatInt(int64(i), 10)] = arg
	}
	m["query"] = query
	span.SetAttributes(toAttributes(m)...)

	row := e.tx.QueryRowContext(ctx, query, args...)
	return row
}

func (e *txExt) PrepareContext(ctx context.Context, query string) (*stmtExt, error) {
	ctx, span := tracer.Start(ctx, "PrepareContext")
	defer span.End()
	span.SetAttributes(toAttributes(map[string]any{
		"query": query,
	})...)

	stmt, err := e.tx.PrepareContext(ctx, query)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &stmtExt{
		stmt: stmt,
	}, nil
}

type stmtExt struct {
	stmt *sql.Stmt
}

func (e *stmtExt) Close() error {
	return errors.WithStack(e.stmt.Close())
}

func (e *stmtExt) QueryRowContext(ctx context.Context, args ...any) *sql.Row {
	ctx, span := tracer.Start(ctx, "QueryRowContext")
	defer span.End()
	m := make(map[string]any)
	for i, arg := range args {
		m["arg"+strconv.FormatInt(int64(i), 10)] = arg
	}
	span.SetAttributes(toAttributes(m)...)

	row := e.stmt.QueryRowContext(ctx, args...)
	return row
}

func (e *stmtExt) ExecContext(ctx context.Context, args ...any) (sql.Result, error) {
	ctx, span := tracer.Start(ctx, "ExecContext")
	defer span.End()
	m := make(map[string]any)
	for i, arg := range args {
		m["arg"+strconv.FormatInt(int64(i), 10)] = arg
	}
	span.SetAttributes(toAttributes(m)...)

	result, err := e.stmt.ExecContext(ctx, args...)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return result, nil
}

type User struct {
	ID           string    `db:"id" json:"id"`
	Name         string    `db:"name" json:"name"`
	Email        string    `db:"email" json:"email"`
	PasswordHash string    `db:"password_hash" json:"-"`
	CreatedAt    time.Time `db:"created_at" json:"created_at"`
	UpdatedAt    time.Time `db:"updated_at" json:"updated_at"`
}

type Article struct {
	ID                 string    `db:"id" json:"id"`
	Title              string    `db:"title" json:"title"`
	Body               string    `db:"body" json:"body"`
	UserID             string    `db:"user_id" json:"user_id"`
	TotalFavoriteCount int       `db:"total_favorite_count" json:"total_favorite_count"`
	CreatedAt          time.Time `db:"created_at" json:"created_at"`
	UpdatedAt          time.Time `db:"updated_at" json:"updated_at"`
}

type UserArticle struct {
	UserID    string    `db:"user_id" json:"user_id"`
	ArticleID string    `db:"article_id" json:"article_id"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}
