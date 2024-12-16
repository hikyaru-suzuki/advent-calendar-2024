package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/labstack/echo/v4"
)

func sleep() {
	time.Sleep(100 * time.Millisecond)
}

func doLoadTestRequest(ctx context.Context, e *echo.Echo, userName, method, path, body string) (*httptest.ResponseRecorder, error) {
	req, err := http.NewRequestWithContext(ctx, method, path, strings.NewReader(body))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.SetBasicAuth(userName+"@email.com", userName)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec, nil
}

type initScenario struct{}

func (s *initScenario) Run(ctx context.Context, e *echo.Echo) ([]string, error) {
	length := 10
	articleIDs := make([]string, 0, length)

	for i := 0; i < length; i++ {
		userName := strconv.FormatInt(time.Now().UnixNano(), 10)

		rec, err := doLoadTestRequest(ctx, e, userName, http.MethodPost, "/user", fmt.Sprintf(`{
		"name": "%s",
		"email": "%s@email.com",
		"password": "%s"
}`, userName, userName, userName))
		if err != nil {
			return nil, errors.WithStack(err)
		}
		if rec.Code != http.StatusOK {
			return nil, errors.Newf("ユーザー登録に失敗しました。: %s", rec.Body.String())
		}

		rec, err = doLoadTestRequest(ctx, e, userName, http.MethodPost, "/article", fmt.Sprintf(`{
	"title": "title_v1 %d by %s",
	"body": "body_v1 %d by %s"
}`, i, userName, i, userName))
		if err != nil {
			return nil, errors.WithStack(err)
		}
		if rec.Code != http.StatusOK {
			return nil, errors.Newf("記事投稿に失敗しました。: %s", rec.Body.String())
		}
		res := &struct {
			ArticleID string `json:"article_id"`
		}{}
		if err := json.Unmarshal(rec.Body.Bytes(), res); err != nil {
			return nil, errors.WithStack(err)
		}
		articleIDs = append(articleIDs, res.ArticleID)
	}

	return articleIDs, nil
}

type userSpawnScenario struct{}

func (s *userSpawnScenario) Run(ctx context.Context, e *echo.Echo) (string, error) {
	userName := strconv.FormatInt(time.Now().UnixNano(), 10)

	rec, err := doLoadTestRequest(ctx, e, userName, http.MethodPost, "/user", fmt.Sprintf(`{
		"name": "%s",
		"email": "%s@email.com",
		"password": "%s"
}`, userName, userName, userName))
	if err != nil {
		return "", errors.WithStack(err)
	}
	if rec.Code != http.StatusOK {
		return "", errors.Newf("ユーザー登録に失敗しました。: %s", rec.Body.String())
	}

	return userName, nil
}

type articleScenario struct {
	randUtil randUtil
}

func (s *articleScenario) Run(ctx context.Context, e *echo.Echo, userName string, initArticleIDs []string) error {
	for i := 0; i < 5; i++ {
		if !s.randUtil.Hit(90, 100) {
			continue
		}
		rec, err := doLoadTestRequest(ctx, e, userName, http.MethodPost, "/article", fmt.Sprintf(`{
	"title": "title_v1 %d by %s",
	"body": "body_v1 %d by %s"
}`, i, userName, i, userName))
		if err != nil {
			return errors.WithStack(err)
		}
		if rec.Code != http.StatusOK {
			return errors.Newf("記事投稿に失敗しました。: %s", rec.Body.String())
		}
		res := &struct {
			ArticleID string `json:"article_id"`
		}{}
		if err := json.Unmarshal(rec.Body.Bytes(), res); err != nil {
			return errors.WithStack(err)
		}
		articleID := res.ArticleID
		sleep()

		/* 記事一覧取得 */
		if s.randUtil.Hit(50, 100) {
			rec, err = doLoadTestRequest(ctx, e, userName, http.MethodGet, "/articles", ``)
			if err != nil {
				return errors.WithStack(err)
			}
			if rec.Code != http.StatusOK {
				return errors.Newf("記事一覧取得に失敗しました。: %s", rec.Body.String())
			}
			sleep()
		}

		/* 記事詳細取得 */
		if s.randUtil.Hit(50, 100) {
			rec, err = doLoadTestRequest(ctx, e, userName, http.MethodGet, "/article/"+articleID, ``)
			if err != nil {
				return errors.WithStack(err)
			}
			if rec.Code != http.StatusOK {
				return errors.Newf("記事詳細取得に失敗しました。: %s", rec.Body.String())
			}
			sleep()
		}

		/* 記事更新 */
		if s.randUtil.Hit(20, 100) {
			rec, err = doLoadTestRequest(ctx, e, userName, http.MethodPatch, "/article/"+articleID, fmt.Sprintf(`{
	"title": "title_v2 %d by %s",
	"body": "body_v2 %d by %s"
}`, i, userName, i, userName))
			if err != nil {
				return errors.WithStack(err)
			}
			if rec.Code != http.StatusOK {
				return errors.Newf("記事更新に失敗しました。: %s", rec.Body.String())
			}
			sleep()
		}

		/* 記事削除 */
		if s.randUtil.Hit(1, 100) {
			rec, err = doLoadTestRequest(ctx, e, userName, http.MethodDelete, "/article/"+articleID, ``)
			if err != nil {
				return errors.WithStack(err)
			}
			if rec.Code != http.StatusOK {
				return errors.Newf("記事削除に失敗しました。: %s", rec.Body.String())
			}
			sleep()
		}
	}

	/* お気に入り一覧取得 */
	if s.randUtil.Hit(30, 100) {
		rec, err := doLoadTestRequest(ctx, e, userName, http.MethodGet, "/favorite/articles", ``)
		if err != nil {
			return errors.WithStack(err)
		}
		if rec.Code != http.StatusOK {
			return errors.Newf("お気に入り一覧取得に失敗しました。: %s", rec.Body.String())
		}
		sleep()
	}

	/* お気に入り登録 */
	if s.randUtil.Hit(50, 100) {
		for _, articleID := range initArticleIDs {
			rec, err := doLoadTestRequest(ctx, e, userName, http.MethodPost, "/favorite/article/"+articleID, ``)
			if err != nil {
				return errors.WithStack(err)
			}
			if rec.Code != http.StatusOK {
				return errors.Newf("お気に入り登録に失敗しました。: %s", rec.Body.String())
			}
			if !s.randUtil.Hit(10, 100) { // 10%の確率で継続する
				continue
			}
			sleep()
		}
	}

	return nil
}
