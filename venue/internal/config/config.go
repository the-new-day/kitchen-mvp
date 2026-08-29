// Package config loads the configuration of the venue service. The venue is a
// system of its own: it keeps its own database and reaches the platform only
// over HTTP and Kafka, so its settings name nothing of the platform except the
// address and the key it was given at onboarding.
package config

import (
	platform "avito-kitchen/internal/config"
	"log/slog"
	"time"
)

// Config holds everything the venue service needs to run.
type Config struct {
	Env       string            `env:"APP_ENV"   envDefault:"dev"`
	LogLevel  slog.Level        `env:"LOG_LEVEL" envDefault:"info"`
	HTTP      platform.HTTP     `envPrefix:"HTTP_"`
	Postgres  platform.Postgres `envPrefix:"POSTGRES_"`
	Kafka     Kafka             `envPrefix:"KAFKA_"`
	Partner   Partner           `envPrefix:"PARTNER_"`
	Autopilot Autopilot         `envPrefix:"AUTOPILOT_"`
	Bootstrap Bootstrap         `envPrefix:"BOOTSTRAP_"`
}

// Kafka is the topic the venue reads its orders from, as it is told at
// onboarding.
type Kafka struct {
	Brokers     []string `env:"BROKERS"      envDefault:"localhost:9092" envSeparator:","`
	OrdersTopic string   `env:"ORDERS_TOPIC" envDefault:"kitchen.orders.v1"`
}

// Partner is how the venue reaches the partner API of the platform.
type Partner struct {
	BaseURL string        `env:"BASE_URL" envDefault:"http://localhost:8081/api/v1"`
	APIKey  string        `env:"API_KEY"  envDefault:"vk_demo_bakery_dev"`
	Timeout time.Duration `env:"TIMEOUT"  envDefault:"10s"`
}

// Autopilot runs the orders through the kitchen without a cook. Every delay is
// counted from the moment the order entered the state it is in.
type Autopilot struct {
	Enabled     bool          `env:"ENABLED"      envDefault:"true"`
	Interval    time.Duration `env:"INTERVAL"     envDefault:"1s"`
	AcceptAfter time.Duration `env:"ACCEPT_AFTER" envDefault:"2s"`
	CookAfter   time.Duration `env:"COOK_AFTER"   envDefault:"2s"`
	ReadyAfter  time.Duration `env:"READY_AFTER"  envDefault:"5s"`
	HandAfter   time.Duration `env:"HAND_AFTER"   envDefault:"3s"`
	BatchSize   int           `env:"BATCH_SIZE"   envDefault:"50"`
}

// Bootstrap is how long the venue waits before offering its menu to the
// platform again. The first attempts are expected to fail: the key of the venue
// reaches the platform with its own seeding step, after the services start.
type Bootstrap struct {
	Retry time.Duration `env:"RETRY" envDefault:"3s"`
}

// Load reads the configuration from the environment.
func Load() (Config, error) {
	var cfg Config

	if err := platform.ParseInto(&cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}
