// Package config loads service configuration from the environment.
package config

import (
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"time"

	env "github.com/caarlos0/env/v11"
)

// Config holds the settings shared by every binary. Every field has a
// default that works with no environment set.
type Config struct {
	Env      string     `env:"APP_ENV"   envDefault:"dev"`
	LogLevel slog.Level `env:"LOG_LEVEL" envDefault:"info"`
	Orders   Orders     `envPrefix:"ORDERS_"`
	HTTP     HTTP       `envPrefix:"HTTP_"`
	Postgres Postgres   `envPrefix:"POSTGRES_"`
	Kafka    Kafka      `envPrefix:"KAFKA_"`
}

// HTTP configures the embedded HTTP server. It sets no write deadline, so
// long-lived responses such as event streams are not cut off.
type HTTP struct {
	Addr              string        `env:"ADDR"                envDefault:":8080"`
	ReadHeaderTimeout time.Duration `env:"READ_HEADER_TIMEOUT" envDefault:"5s"`
	IdleTimeout       time.Duration `env:"IDLE_TIMEOUT"        envDefault:"60s"`
	ShutdownTimeout   time.Duration `env:"SHUTDOWN_TIMEOUT"    envDefault:"15s"`
}

// Postgres configures the connection pool to the platform database.
type Postgres struct {
	DSN            string        `env:"DSN"             envDefault:"postgres://kitchen:kitchen@localhost:5432/kitchen?sslmode=disable"`
	MaxConns       int32         `env:"MAX_CONNS"       envDefault:"10"`
	ConnectTimeout time.Duration `env:"CONNECT_TIMEOUT" envDefault:"5s"`
}

// Kafka names the topics of the platform. The topic a venue reads its orders
// from is part of what it is told at onboarding.
type Kafka struct {
	OrdersTopic string `env:"ORDERS_TOPIC" envDefault:"kitchen.orders.v1"`
}

// Orders configures the placing of orders.
type Orders struct {
	// IdempotencyTTL is how long the answer to an attempt of placing an order
	// is kept to be given back to a repeat of it.
	IdempotencyTTL time.Duration `env:"IDEMPOTENCY_TTL" envDefault:"24h"`
}

// Load reads the configuration from the environment.
func Load() (Config, error) {
	var cfg Config

	opts := env.Options{
		FuncMap: map[reflect.Type]env.ParserFunc{
			reflect.TypeFor[slog.Level](): parseLogLevel,
		},
	}
	if err := env.ParseWithOptions(&cfg, opts); err != nil {
		return Config{}, fmt.Errorf("parse environment: %w", err)
	}

	return cfg, nil
}

func parseLogLevel(v string) (any, error) {
	var level slog.Level
	if err := level.UnmarshalText([]byte(strings.TrimSpace(v))); err != nil {
		return nil, fmt.Errorf("unknown log level %q: %w", v, err)
	}

	return level, nil
}
