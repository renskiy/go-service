package app

import (
	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	GRPCPort string `envconfig:"GRPC_PORT" default:":9090"`
	HTTPPort string `envconfig:"HTTP_PORT" default:":8080"`
}

func NewConfigFromEnv() (cfg Config, err error) {
	err = envconfig.Process("", &cfg)
	return cfg, err
}
