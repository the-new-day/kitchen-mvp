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
	Worker   Worker
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
// from is part of what it is told at onboarding; the status topic is read by
// the platform itself.
type Kafka struct {
	Brokers     []string      `env:"BROKERS"          envDefault:"localhost:9092" envSeparator:","`
	OrdersTopic string        `env:"ORDERS_TOPIC"     envDefault:"kitchen.orders.v1"`
	StatusTopic string        `env:"STATUS_TOPIC"     envDefault:"kitchen.order-status.v1"`
	Partitions  int           `env:"TOPIC_PARTITIONS" envDefault:"3"`
	Timeout     time.Duration `env:"TIMEOUT"     envDefault:"10s"`
}

// Topics returns every topic the platform publishes to.
func (k Kafka) Topics() []string {
	return []string{k.OrdersTopic, k.StatusTopic}
}

// Worker configures the background jobs. The names of the variables are the
// knobs of the deployment and are spelled out rather than prefixed.
type Worker struct {
	Outbox        OutboxJob
	Reaper        ReaperJob
	IdempotencyGC IdempotencyGCJob
}

// OutboxJob configures the publishing of the accumulated events. PollMin is
// the pause after a partial batch, PollMax the longest pause on an idle table:
// it is the only setting that decides how long an event waits to be delivered.
type OutboxJob struct {
	BatchSize int           `env:"OUTBOX_BATCH_SIZE" envDefault:"100"`
	PollMin   time.Duration `env:"OUTBOX_POLL_MIN"   envDefault:"50ms"`
	PollMax   time.Duration `env:"OUTBOX_POLL_MAX"   envDefault:"250ms"`
}

// ReaperJob configures the automatic rejection of the orders no venue has
// taken into work. AcceptTimeout is how long an order waits for its venue.
type ReaperJob struct {
	AcceptTimeout time.Duration `env:"ORDER_ACCEPT_TIMEOUT"    envDefault:"5m"`
	Interval      time.Duration `env:"ORDER_REAPER_INTERVAL"   envDefault:"15s"`
	BatchSize     int           `env:"ORDER_REAPER_BATCH_SIZE" envDefault:"50"`
}

// IdempotencyGCJob configures the collection of the expired idempotency keys.
type IdempotencyGCJob struct {
	Interval  time.Duration `env:"IDEMPOTENCY_GC_INTERVAL"   envDefault:"10m"`
	BatchSize int           `env:"IDEMPOTENCY_GC_BATCH_SIZE" envDefault:"1000"`
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

	if err := ParseInto(&cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

// ParseInto fills target from the environment. It is what Load is built on and
// what a service carrying a configuration of its own parses through, so that
// every binary of the repository reads the same value the same way.
func ParseInto(target any) error {
	opts := env.Options{
		FuncMap: map[reflect.Type]env.ParserFunc{
			reflect.TypeFor[slog.Level](): parseLogLevel,
		},
	}

	if err := env.ParseWithOptions(target, opts); err != nil {
		return fmt.Errorf("parse environment: %w", err)
	}

	return nil
}

func parseLogLevel(v string) (any, error) {
	var level slog.Level
	if err := level.UnmarshalText([]byte(strings.TrimSpace(v))); err != nil {
		return nil, fmt.Errorf("unknown log level %q: %w", v, err)
	}

	return level, nil
}
