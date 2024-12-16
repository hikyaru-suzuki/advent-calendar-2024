package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// テストケースは直列前提
var h = &handler{
	db:       nil,
	randUtil: nil,
}

type randImplMock struct {
	mock.Mock
}

func (r *randImplMock) NewString() string {
	args := r.Called()
	return args.String(0)
}

func (r *randImplMock) Hit(rate, denominator int) bool {
	return true
}

type timerImplMock struct {
	mock.Mock
}

func (t *timerImplMock) Now() time.Time {
	args := t.Called()
	return args.Get(0).(time.Time)
}

func TestMain(m *testing.M) {
	if err := testMain(); err != nil {
		log.Printf("%+v\n", err)
		os.Exit(1)
		return
	}
	os.Exit(m.Run())
}

func testMain() error {
	ctx := context.Background()

	conn, err := newConnection("127.0.0.1", "", "postgres", "postgres", "", "disable")
	if err != nil {
		return errors.WithStack(err)
	}
	if _, err := conn.ExecContext(ctx, "DROP DATABASE IF EXISTS blog_test"); err != nil {
		return errors.WithStack(err)
	}
	if _, err := conn.ExecContext(ctx, "CREATE DATABASE blog_test"); err != nil {
		return errors.WithStack(err)
	}
	if err := conn.Close(); err != nil {
		return errors.WithStack(err)
	}

	conn, err = newConnection("127.0.0.1", "", "postgres", "postgres", "blog_test", "disable")
	if err != nil {
		return errors.WithStack(err)
	}
	sqlFile, err := os.ReadFile("./ddl.sql")
	if err != nil {
		return errors.WithStack(err)
	}
	if _, err := conn.ExecContext(ctx, string(sqlFile)); err != nil {
		return errors.WithStack(err)
	}
	h.db = &dbExt{conn}

	return nil
}

var baseTime = time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)

var userName = strconv.FormatInt(baseTime.UnixNano(), 10)

func doTestRequest(ctx context.Context, e *echo.Echo, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequestWithContext(ctx, method, path, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.SetBasicAuth(userName+"@email.com", userName)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func Test_E2E(t *testing.T) {
	ctx := context.Background()
	e := setupEcho(h)

	var rec *httptest.ResponseRecorder

	randImplMockInstance := &randImplMock{}
	h.randUtil = randImplMockInstance
	timerImplMockInstance := &timerImplMock{}
	h.timer = timerImplMockInstance
	defer func() {
		randImplMockInstance.AssertExpectations(t)
		timerImplMockInstance.AssertExpectations(t)
	}()

	/* ユーザー登録 */
	randImplMockInstance.On("NewString").Return("2f2812ce-4511-4095-a144-2cefcb120e62").Times(1)
	timerImplMockInstance.On("Now").Return(baseTime.Add(1 * time.Millisecond).UTC()).Times(2)
	rec = doTestRequest(ctx, e, http.MethodPost, "/user", fmt.Sprintf(`{
		"name": "%s",
		"email": "%s@email.com",
		"password": "%s"
}`, userName, userName, userName))
	require.Equal(t, http.StatusOK, rec.Code)

	/* 記事作成1 */
	randImplMockInstance.On("NewString").Return("cd4b2c08-3387-5238-bcf8-0b9f0b87e8ac").Times(1)
	createdAt1 := baseTime.Add(21 * time.Millisecond).In(time.UTC)
	updateAt1 := baseTime.Add(22 * time.Millisecond).In(time.UTC)
	timerImplMockInstance.On("Now").Return(createdAt1).Times(1)
	timerImplMockInstance.On("Now").Return(updateAt1).Times(1)
	rec = doTestRequest(ctx, e, http.MethodPost, "/article", fmt.Sprintf(`{
		"title": "title1",
		"body": "body1"
}`))
	require.JSONEq(t, rec.Body.String(), fmt.Sprintf(`{
	"article_id": "cd4b2c08-3387-5238-bcf8-0b9f0b87e8ac"
}`))
	require.Equal(t, http.StatusOK, rec.Code)

	/* 記事作成2 */
	randImplMockInstance.On("NewString").Return("5847a07c-84bd-7eda-9fad-b44c70bc9ffa").Times(1)
	createdAt2 := baseTime.Add(31 * time.Millisecond).In(time.UTC)
	updateAt2 := baseTime.Add(32 * time.Millisecond).In(time.UTC)
	timerImplMockInstance.On("Now").Return(createdAt2).Times(1)
	timerImplMockInstance.On("Now").Return(updateAt2).Times(1)
	rec = doTestRequest(ctx, e, http.MethodPost, "/article", fmt.Sprintf(`{
		"title": "title2",
		"body": "body2"
}`))
	require.JSONEq(t, rec.Body.String(), fmt.Sprintf(`{
	"article_id": "5847a07c-84bd-7eda-9fad-b44c70bc9ffa"
}`))
	require.Equal(t, http.StatusOK, rec.Code)

	/* 記事一覧取得 */
	rec = doTestRequest(ctx, e, http.MethodGet, "/articles", ``)
	require.JSONEq(t, rec.Body.String(), `{
	"list": [
		{	
			"article_id": "5847a07c-84bd-7eda-9fad-b44c70bc9ffa",
			"title": "title2"
		},
		{	
			"article_id": "cd4b2c08-3387-5238-bcf8-0b9f0b87e8ac",
			"title": "title1"
		}
	]
}
`)
	require.Equal(t, http.StatusOK, rec.Code)

	/* 記事詳細取得 */
	rec = doTestRequest(ctx, e, http.MethodGet, "/article/5847a07c-84bd-7eda-9fad-b44c70bc9ffa", ``)
	require.JSONEq(t, rec.Body.String(), fmt.Sprintf(`{
	"id": "5847a07c-84bd-7eda-9fad-b44c70bc9ffa",
	"title": "title2",
	"body": "body2",
	"user_id": "2f2812ce-4511-4095-a144-2cefcb120e62",
	"total_favorite_count": 0,
	"created_at": "%s",
	"updated_at": "%s"
}
`, createdAt2.Format(time.RFC3339Nano), updateAt2.Format(time.RFC3339Nano)))

	/* 記事更新 */
	updateAt3 := baseTime.Add(42 * time.Millisecond).In(time.UTC)
	timerImplMockInstance.On("Now").Return(updateAt3).Times(1)
	rec = doTestRequest(ctx, e, http.MethodPatch, "/article/5847a07c-84bd-7eda-9fad-b44c70bc9ffa", `{
	"title": "title2-1",
	"body": "body2-1"
}`)
	require.Equal(t, http.StatusOK, rec.Code)
	rec = doTestRequest(ctx, e, http.MethodGet, "/article/5847a07c-84bd-7eda-9fad-b44c70bc9ffa", ``)
	require.JSONEq(t, rec.Body.String(), fmt.Sprintf(`{
	"id": "5847a07c-84bd-7eda-9fad-b44c70bc9ffa",
	"title": "title2-1",
	"body": "body2-1",
	"user_id": "2f2812ce-4511-4095-a144-2cefcb120e62",
	"total_favorite_count": 0,
	"created_at": "%s",
	"updated_at": "%s"
}`, createdAt2.Format(time.RFC3339Nano), updateAt3.Format(time.RFC3339Nano)))

	/* 記事削除 */
	rec = doTestRequest(ctx, e, http.MethodDelete, "/article/5847a07c-84bd-7eda-9fad-b44c70bc9ffa", ``)
	require.Equal(t, http.StatusOK, rec.Code)
	rec = doTestRequest(ctx, e, http.MethodGet, "/articles", ``)
	require.JSONEq(t, rec.Body.String(), `{
	"list": [
		{	
			"article_id": "cd4b2c08-3387-5238-bcf8-0b9f0b87e8ac",
			"title": "title1"
		}
	]
}`)

	/* お気に入り登録 */
	createdAt4 := baseTime.Add(51 * time.Millisecond).In(time.UTC)
	updatedAt4 := baseTime.Add(52 * time.Millisecond).In(time.UTC)
	timerImplMockInstance.On("Now").Return(updatedAt4).Times(1)
	timerImplMockInstance.On("Now").Return(createdAt4).Times(1)
	timerImplMockInstance.On("Now").Return(updatedAt4).Times(1)
	rec = doTestRequest(ctx, e, http.MethodPost, "/favorite/article/cd4b2c08-3387-5238-bcf8-0b9f0b87e8ac", ``)
	require.Equal(t, http.StatusOK, rec.Code)
	rec = doTestRequest(ctx, e, http.MethodGet, "/article/cd4b2c08-3387-5238-bcf8-0b9f0b87e8ac", ``)
	require.JSONEq(t, rec.Body.String(), fmt.Sprintf(`{
	"id": "cd4b2c08-3387-5238-bcf8-0b9f0b87e8ac",
	"title": "title1",
	"body": "body1",
	"user_id": "2f2812ce-4511-4095-a144-2cefcb120e62",
	"total_favorite_count": 1,
	"created_at": "%s",
	"updated_at": "%s"
}`, createdAt1.Format(time.RFC3339Nano), updatedAt4.Format(time.RFC3339Nano)))

	/* お気に入り一覧取得 */
	rec = doTestRequest(ctx, e, http.MethodGet, "/favorite/articles", ``)
	require.JSONEq(t, rec.Body.String(), `{
	"list": [
		{	
			"article_id": "cd4b2c08-3387-5238-bcf8-0b9f0b87e8ac",
			"title": "title1"
		}
	]
}`)
}
