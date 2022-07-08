package service

import (
	"context"

	"go-service/internal/app"
	"go-service/internal/service/repository"
	"go-service/pkg/service/server"
)

type service struct {
	*app.App
	server.UnimplementedServiceServer
	repo *repository.Repository
}

func New(app *app.App) server.ServiceServer {
	return &service{
		App:  app,
		repo: repository.New(app.DB()),
	}
}

func (s *service) Add(ctx context.Context, request *server.AddRequest) (*server.AddResponse, error) {
	return nil, nil
}
