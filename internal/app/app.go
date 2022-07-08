package app

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"

	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/multierr"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type backgroundJob func(context.Context) error

func New(ctx context.Context, cfg Config) (*App, error) {
	app := &App{
		ctx: ctx,
		cfg: cfg,
	}
	app.registerGRPCServer()
	app.registerHTTPServer()
	return app, nil
}

type App struct {
	ctx    context.Context
	logger *zap.Logger
	db     *sqlx.DB
	cfg    Config
	grpc   *grpc.Server
	jobs   []backgroundJob
}

func (app *App) Run() error {
	app.Logger().Info("started application")

	var wg sync.WaitGroup
	errChannel := make(chan error, len(app.jobs))

	for _, job := range app.jobs {
		wg.Add(1)
		go func(job backgroundJob) {
			defer wg.Done()
			errChannel <- job(app.ctx)
		}(job)
	}

	select {
	case <-app.ctx.Done():
		wg.Wait()
		close(errChannel)
		errs := make([]error, len(errChannel))
		for err := range errChannel {
			errs = append(errs, err)
		}
		return multierr.Combine(errs...)
	case err := <-errChannel:
		return err
	}
}

func (app *App) Error(code codes.Code, err error, fields ...zap.Field) error {
	if err == nil {
		return nil
	}
	if grpcStatus, ok := status.FromError(err); ok {
		// do not wrap GRPC errors
		return grpcStatus.Err()
	}

	var msg string
	switch code {
	case codes.Internal, codes.Unimplemented:
		fields = append(fields, zap.Error(err))
		msg = fmt.Sprintf("%s error", code)
		app.logger.Error(msg, fields...)
	default:
		msg = fmt.Sprintf("%s: %s", code, err.Error())
		app.logger.Warn(msg, fields...)
	}

	return status.Error(code, msg)
}

func (app *App) RegisterService(desc *grpc.ServiceDesc, impl interface{}) {
	app.grpc.RegisterService(desc, impl)
}

func (app *App) AddBackgroundJob(job backgroundJob) {
	app.jobs = append(app.jobs, job)
}

func (app *App) Logger() *zap.Logger {
	return app.logger
}

func (app *App) DB() *sqlx.DB {
	return app.db
}

func (app *App) GRPC() *grpc.Server {
	return app.grpc
}

func (app *App) registerGRPCServer() {
	app.grpc = grpc.NewServer()
	app.AddBackgroundJob(func(ctx context.Context) error {
		listener, listenErr := net.Listen("tcp", app.cfg.GRPCPort)
		if listenErr != nil {
			return errors.Wrap(listenErr, "could not open GRPC port to serve")
		}
		app.Logger().Info("starting GRPC server")
		return errors.Wrap(app.grpc.Serve(listener), "GRPC server error")
	})
	app.AddBackgroundJob(func(ctx context.Context) error {
		<-ctx.Done()
		app.grpc.GracefulStop()
		return nil
	})
}

func (app *App) registerHTTPServer() {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if err := app.db.Ping(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
	httpServer := &http.Server{
		Handler: mux,
		Addr:    app.cfg.HTTPPort,
	}
	app.AddBackgroundJob(func(ctx context.Context) error {
		app.Logger().Info("starting HTTP server")
		if err := httpServer.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	})
	app.AddBackgroundJob(func(ctx context.Context) error {
		<-ctx.Done()
		return httpServer.Shutdown(context.Background())
	})
}
