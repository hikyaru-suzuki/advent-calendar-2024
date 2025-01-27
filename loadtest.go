package main

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/labstack/echo/v4"
)

type config struct {
	Duration  time.Duration // 試験実行時間(second)
	Users     int32         // 同時実行ユーザー数
	SpawnRate int32         // ユーザーの増加率 (SpawnRate/per second)
}

func runLoadTest(
	ctx context.Context,
	conf *config,
	e *echo.Echo,
	initScenario *initScenario,
	userSpawnScenario *userSpawnScenario,
	articleScenario *articleScenario,
) error {
	log.Println("初期化シナリオを実行します。")
	articleIDs, err := initScenario.Run(ctx, e)
	if err != nil {
		return errors.WithStack(err)
	}
	log.Println("初期化シナリオを実行しました。")

	var workers sync.WaitGroup
	defer workers.Wait()
	start := time.Now()
	// 負荷試験の終了処理をcontextで行うが、シナリオ実行中にcontext cancelが走ると通信エラーになるのでシナリオにはこのtctxを渡さない
	tctx, cancel := context.WithTimeout(ctx, conf.Duration)
	defer cancel()
	users := make(chan struct{}, conf.Users)
	defer close(users)

	go func() {
		for {
			// 5秒おきにデバッグログ出力
			time.Sleep(5 * time.Second)
			log.Printf("実行時間: %s. 同時並列数: %d", time.Since(start), len(users))
			select {
			case <-tctx.Done():
				return
			default:
			}
		}
	}()

	log.Println("負荷試験を開始します。")
	defer log.Println("負荷試験が完了しました。")
	for {
		// 現在のユーザー数に達するのにかかる時間-現在の経過時間
		// ex) 秒間10ユーザーの設定、15.5秒経過していて160ユーザーいる場合、
		//     本来160ユーザーは16秒経過時にいるべき人数なので進みすぎている分を待機する
		//     160*(1/10)s - 15.5s = 0.5s
		interval := time.Second.Nanoseconds() / int64(conf.SpawnRate)
		time.Sleep(time.Duration(int64(len(users))*interval) - time.Since(start))

		select {
		case users <- struct{}{}:
			// tctx.Doneとusersチャネルの解放が同時に発生しているときselectではランダムな選択になるため、
			// 再度tctx.Doneになっていないかチェックする
			select {
			case <-tctx.Done():
				// 負荷試験を終了する
				return nil
			default:
				// 継続
			}

			workers.Add(1)
			go func() {
				defer func() {
					<-users
					workers.Done()
				}()

				reqCtx := context.Background()
				userName, err := userSpawnScenario.Run(reqCtx, e)
				if err != nil {
					errorHandler(ctx, err)
					return
				}

				// シナリオ実行
				select {
				case <-reqCtx.Done():
					// シナリオを中断する
					return
				case <-tctx.Done():
					// シナリオを中断する
					return
				default:
					// シナリオ実行
					if err := articleScenario.Run(reqCtx, e, userName, articleIDs); err != nil {
						// エラーが飛んできたらこのユーザーのシナリオは終了する
						errorHandler(reqCtx, err)
						return
					}
				}
			}()
		case <-tctx.Done():
			// 負荷試験を終了する
			return nil
		}
	}
}

func errorHandler(ctx context.Context, err error) {
	// cancelによるエラーはクライアント側の正常な終了処理
	if errors.Is(err, context.Canceled) {
		return
	}

	// log.Printf("エラーが発生しました。: %+v\n", err)
}
