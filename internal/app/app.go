package app

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/multierr"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type requestIDContextKey struct{}

type backgroundJob func(context.Context) error

type App interface {
	Logger() *zap.Logger
	AddBackgroundJob(backgroundJob)
	Run() error
	Error(ctx context.Context, err error, code codes.Code, fields ...zap.Field) error

	RequestID(ctx context.Context) string
}

func New(ctx context.Context, cfg Config) (*app, error) {
	application := &app{
		ctx: ctx,
		cfg: cfg,
	}
	application.registerGRPCServer()
	application.registerHTTPServer()
	return application, nil
}

type app struct {
	ctx    context.Context
	logger *zap.Logger
	cfg    Config
	grpc   *grpc.Server
	jobs   []backgroundJob
}

func (app *app) Run() error {
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

func (app *app) Error(ctx context.Context, err error, code codes.Code, fields ...zap.Field) error {
	if err == nil {
		return nil
	}

	fields = append(fields, zap.Error(err))
	message := fmt.Sprintf("%s: %s", code, err.Error())

	switch code {
	case codes.Internal, codes.Unimplemented:
		app.Logger().Error(message, fields...)
	default:
		app.Logger().Warn(message, fields...)
	}

	if grpcStatus, ok := status.FromError(err); ok {
		// return GRPC errors as is
		return grpcStatus.Err()
	}

	return status.Error(code, message)
}

func (app *app) RegisterService(desc *grpc.ServiceDesc, impl interface{}) {
	app.grpc.RegisterService(desc, impl)
}

func (app *app) AddBackgroundJob(job backgroundJob) {
	app.jobs = append(app.jobs, job)
}

func (app *app) GRPC() *grpc.Server {
	return app.grpc
}

func (app *app) Logger() *zap.Logger {
	return app.logger
}

func (app *app) RequestID(ctx context.Context) string {
	if headers, ok := metadata.FromIncomingContext(ctx); ok {
		if header, ok := headers["x-request-id"]; ok && len(header) > 0 {
			return header[0]
		}
	}
	return ""
}

func (app *app) registerGRPCServer() {
	app.grpc = grpc.NewServer(grpc.ChainUnaryInterceptor(grpcUnaryServerInterceptor))
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

func (app *app) registerHTTPServer() {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		// TODO check DB
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

func grpcUnaryServerInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	var requestID string
	if headers, ok := metadata.FromIncomingContext(ctx); ok {
		if header, ok := headers["x-request-id"]; ok && len(header) > 0 {
			requestID = header[0]
		}
	}
	if requestID == "" {
		requestID = uuid.New().String()
	}
	ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs("x-request-id", requestID))
	ctx = context.WithValue(ctx, requestIDContextKey{}, requestID)
	return handler(ctx, req)
}
