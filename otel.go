package main

import (
	"context"
	"time"

	"github.com/cockroachdb/errors"
	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

const name = "github.com/hikyaru-suzuki/advent-calendar-2024"

var (
	tracer = otel.Tracer(name)
	meter  = otel.Meter(name)
	logger = otelslog.NewLogger(name)
)

// setupOTelSDK bootstraps the OpenTelemetry pipeline.
// If it does not return an error, make sure to call shutdown for proper cleanup.
func setupOTelSDK(ctx context.Context) (shutdown func(context.Context) error, err error) {
	var shutdownFuncs []func(context.Context) error

	// shutdown calls cleanup functions registered via shutdownFuncs.
	// The errors from the calls are joined.
	// Each registered cleanup will be invoked once.
	shutdown = func(ctx context.Context) error {
		var err error
		for _, fn := range shutdownFuncs {
			err = errors.Join(err, fn(ctx))
		}
		shutdownFuncs = nil
		return errors.WithStack(err)
	}

	// handleErr calls shutdown for cleanup and makes sure that all errors are returned.
	handleErr := func(inErr error) {
		err = errors.Join(inErr, shutdown(ctx))
	}

	// Set up propagator.
	prop := newPropagator()
	otel.SetTextMapPropagator(prop)

	// Create resource.
	res, err := newResource()
	if err != nil {
		handleErr(err)
		return
	}

	// Set up trace provider.
	tracerProvider, err := newTraceProvider(ctx, res)
	if err != nil {
		handleErr(err)
		return
	}
	shutdownFuncs = append(shutdownFuncs, tracerProvider.Shutdown)
	otel.SetTracerProvider(tracerProvider)

	// Set up meter provider.
	meterProvider, err := newMeterProvider(ctx, res)
	if err != nil {
		handleErr(err)
		return
	}
	shutdownFuncs = append(shutdownFuncs, meterProvider.Shutdown)
	otel.SetMeterProvider(meterProvider)

	// Set up logger provider.
	loggerProvider, err := newLoggerProvider(ctx, res)
	if err != nil {
		handleErr(err)
		return
	}
	shutdownFuncs = append(shutdownFuncs, loggerProvider.Shutdown)
	global.SetLoggerProvider(loggerProvider)

	return
}

func newResource() (*resource.Resource, error) {
	return resource.Merge(resource.Default(),
		resource.NewWithAttributes(semconv.SchemaURL,
			semconv.ServiceName("blog"),
			semconv.ServiceVersion("0.1.0"),
		))
}

func newPropagator() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
}

func newTraceProvider(ctx context.Context, res *resource.Resource) (*trace.TracerProvider, error) {
	traceExporter, err := stdouttrace.New(
		stdouttrace.WithPrettyPrint())
	if err != nil {
		return nil, errors.WithStack(err)
	}

	httpExporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	_ = traceExporter
	traceProvider := trace.NewTracerProvider(
		//trace.WithBatcher(traceExporter,
		//	// Default is 5s. Set to 1s for demonstrative purposes.
		//	trace.WithBatchTimeout(time.Second)),
		trace.WithResource(res),
		trace.WithBatcher(httpExporter,
			// Default is 5s. Set to 1s for demonstrative purposes.
			trace.WithBatchTimeout(time.Second)),
	)
	return traceProvider, nil
}

func newMeterProvider(ctx context.Context, res *resource.Resource) (*metric.MeterProvider, error) {
	//stdoutExporter, err := stdoutmetric.New()
	//if err != nil {
	//	return nil, errors.WithStack(err)
	//}

	httpExporter, err := otlpmetrichttp.New(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	meterProvider := metric.NewMeterProvider(
		//metric.WithReader(metric.NewPeriodicReader(stdoutExporter,
		//	// Default is 1m. Set to 3s for demonstrative purposes.
		//	metric.WithInterval(3*time.Second))),
		metric.WithResource(res),
		metric.WithReader(metric.NewPeriodicReader(httpExporter,
			// Default is 1m. Set to 3s for demonstrative purposes.
			metric.WithInterval(3*time.Second))),
	)
	return meterProvider, nil
}

func newLoggerProvider(ctx context.Context, res *resource.Resource) (*log.LoggerProvider, error) {
	//stdoutExporter, err := stdoutlog.New()
	//if err != nil {
	//	return nil, errors.WithStack(err)
	//}
	httpExporter, err := otlploghttp.New(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	loggerProvider := log.NewLoggerProvider(
		//log.WithProcessor(log.NewBatchProcessor(stdoutExporter)),
		log.WithResource(res),
		log.WithProcessor(log.NewBatchProcessor(httpExporter)),
	)
	return loggerProvider, nil
}
