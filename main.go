package main

import (
	"context"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"time"

	"github.com/cockroachdb/errors"
	_ "github.com/lib/pq"
)

func main() {
	log.Println("Starting...")
	if err := run(os.Getenv("APP_IS_SERVER_MODE") == "true"); err != nil {
		log.Printf("%+v\n", err)
		os.Exit(1)
	}
	log.Println("Stopped.")
	os.Exit(0)
}

func run(isServerMode bool) error {
	// Handle SIGINT (CTRL+C) gracefully.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// Set up OpenTelemetry.
	otelShutdown, err := setupOTelSDK(ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	// Handle shutdown properly so nothing leaks.
	defer func() {
		err = errors.Join(err, otelShutdown(context.Background()))
	}()

	if !isServerMode {
		conn, err := newConnection(
			os.Getenv("DB_HOST"), os.Getenv("DB_PORT"), os.Getenv("DB_USER"),
			os.Getenv("DB_PASS"), os.Getenv("DB_NAME"), os.Getenv("DB_SSL"),
		)
		if err != nil {
			return errors.WithStack(err)
		}
		log.Println("Ping to DB.")
		if err := conn.PingContext(ctx); err != nil {
			return errors.WithStack(err)
		}
		log.Println("Connected to DB.")
		randUtilImplInstance := &randUtilImpl{
			Rand: rand.New(rand.NewSource(time.Now().UnixNano())),
		}
		h := &handler{
			db:       &dbExt{conn},
			randUtil: randUtilImplInstance,
			timer:    &timerImpl{},
		}

		e := setupEcho(h)

		duration, err := strconv.ParseInt(os.Getenv("APP_DURATION"), 10, 64)
		if err != nil {
			return errors.WithStack(err)
		}
		users, err := strconv.ParseInt(os.Getenv("APP_USERS"), 10, 64)
		if err != nil {
			return errors.WithStack(err)
		}
		spawnRate, err := strconv.ParseInt(os.Getenv("APP_SPAWN_RATE"), 10, 64)
		if err != nil {
			return errors.WithStack(err)
		}

		loadErr := make(chan error, 1)
		go func() {
			loadErr <- runLoadTest(
				ctx,
				&config{
					Duration:  time.Duration(duration) * time.Second,
					Users:     int32(users),
					SpawnRate: int32(spawnRate),
				},
				e,
				&initScenario{},
				&userSpawnScenario{},
				&articleScenario{
					randUtil: randUtilImplInstance,
				},
			)
		}()

		// Wait for interruption.
		select {
		case err = <-loadErr:
			// Error when starting HTTP server.
			return errors.WithStack(err)
		case <-ctx.Done():
			// Wait for first CTRL+C.
			// Stop receiving signal notifications as soon as possible.
			stop()
		}
		return nil
	}

	// Start HTTP server.
	h, err := newHTTPHandler()
	if err != nil {
		return errors.WithStack(err)
	}

	srv := &http.Server{
		Addr:         ":8080",
		BaseContext:  func(_ net.Listener) context.Context { return ctx },
		ReadTimeout:  time.Second,
		WriteTimeout: 10 * time.Second,
		Handler:      h,
	}

	srvErr := make(chan error, 1)
	go func() {
		srvErr <- srv.ListenAndServe()
	}()

	// Wait for interruption.
	select {
	case err = <-srvErr:
		// Error when starting HTTP server.
		return errors.WithStack(err)
	case <-ctx.Done():
		// Wait for first CTRL+C.
		// Stop receiving signal notifications as soon as possible.
		stop()
	}

	// When Shutdown is called, ListenAndServe immediately returns ErrServerClosed.
	err = srv.Shutdown(context.Background())
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}
