package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"go-service/internal/app"
	"go-service/internal/service"
	"go-service/pkg/service/server"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	config, err := app.NewConfigFromEnv()
	if err != nil {
		log.Fatalf("can't create new config: %s", err)
	}

	application, err := app.New(ctx, config)
	if err != nil {
		log.Fatalf("application could not been initialized: %s", err)
	}

	server.RegisterServiceServer(application.GRPC(), service.New(application))

	if err = application.Run(); err != nil {
		log.Fatalf("application terminated abnormally: %s", err)
	}
}
