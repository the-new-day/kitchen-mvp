// Package integration_test runs the platform whole: a real Postgres and a
// real Redpanda in containers, the HTTP API, the background jobs and the
// consumer of the statuses in the process of the test. What it checks is the
// path an order takes between all of them, which no unit test can see.
package integration_test

import (
	broker "avito-kitchen/internal/broker/kafka"
	"avito-kitchen/internal/config"
	"avito-kitchen/internal/consumer"
	"avito-kitchen/internal/platform/httpx"
	"avito-kitchen/internal/repo/postgres"
	"avito-kitchen/internal/transport/http/kitchen"
	"avito-kitchen/internal/transport/http/partner"
	"avito-kitchen/internal/transport/sse"
	cartusecase "avito-kitchen/internal/usecase/cart"
	catalogusecase "avito-kitchen/internal/usecase/catalog"
	orderusecase "avito-kitchen/internal/usecase/order"
	partnerusecase "avito-kitchen/internal/usecase/partner"
	"avito-kitchen/internal/worker"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/testcontainers/testcontainers-go"
	pgcontainer "github.com/testcontainers/testcontainers-go/modules/postgres"
	rpcontainer "github.com/testcontainers/testcontainers-go/modules/redpanda"
)

// Images the stand is built out of: the very ones the compose of the project
// runs, so the test and the stand differ in nothing but their lifetime.
const (
	postgresImage = "postgres:17-alpine"
	redpandaImage = "redpandadata/redpanda:v24.3.6"
)

// The venue the test orders from and the dishes it sells. The espresso never
// runs out, the cheesecake is counted: the refusals need both kinds.
const (
	venueKey   = "vk_integration_dev"
	espresso   = "SKU-ESPRESSO"
	cheesecake = "SKU-CHEESECAKE"
)

// What the venue costs its customers.
const (
	deliveryFee     = 9_900
	minimumOrder    = 20_000
	espressoPrice   = 15_000
	cheesecakePrice = 27_000
	cheesecakeStock = 5
)

// Timings of the stand: the ones the platform runs with, only shortened —
// nothing here waits for a human to look at it.
const (
	deliveryDuration = time.Second
	jobInterval      = 200 * time.Millisecond
	pollMin          = 20 * time.Millisecond
	pollMax          = 50 * time.Millisecond
)

// stand is the platform as one test run sees it.
type stand struct {
	baseURL string
	dsn     string
	topics  config.Kafka
	venueID uuid.UUID
	items   map[string]uuid.UUID
	stop    func()
}

// containers are the servers the platform stands on.
type containers struct {
	dsn     string
	brokers string
	down    func()
}

// start brings the whole platform up and returns it with the way to take it
// down again.
func start(ctx context.Context) (*stand, error) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	servers, err := infrastructure(ctx)
	if err != nil {
		return nil, err
	}

	venueID, items, err := fixture(ctx, servers.dsn)
	if err != nil {
		servers.down()

		return nil, err
	}

	topics := config.Kafka{
		Brokers:     []string{servers.brokers},
		OrdersTopic: "kitchen.orders.v1",
		StatusTopic: "kitchen.order-status.v1",
		Partitions:  1,
		Timeout:     10 * time.Second,
	}

	if err := broker.EnsureTopics(ctx, topics); err != nil {
		servers.down()

		return nil, fmt.Errorf("create topics: %w", err)
	}

	pool, err := postgres.New(ctx, config.Postgres{
		DSN:            servers.dsn,
		MaxConns:       10,
		ConnectTimeout: 5 * time.Second,
	}, log)
	if err != nil {
		servers.down()

		return nil, fmt.Errorf("connect to the database: %w", err)
	}

	orders := orderService(pool, topics)
	hub := sse.NewHub(16, log)
	server := httptest.NewServer(mount(pool, orders, hub, topics, log))

	background, stopBackground := context.WithCancel(context.Background())
	producer := broker.NewProducer(topics)

	var running sync.WaitGroup

	running.Go(func() {
		worker.Run(background, log, jobs(pool, orders, producer, log)...)
	})

	running.Go(func() {
		follow(background, topics, hub, log)
	})

	stop := func() {
		stopBackground()
		running.Wait()
		_ = producer.Close()
		server.Close()
		pool.Close()
		servers.down()
	}

	return &stand{
		baseURL: server.URL + "/api/v1",
		dsn:     servers.dsn,
		topics:  topics,
		venueID: venueID,
		items:   items,
		stop:    stop,
	}, nil
}

// infrastructure starts the containers and prepares the schema on them.
func infrastructure(ctx context.Context) (containers, error) {
	pg, err := pgcontainer.Run(ctx, postgresImage,
		pgcontainer.WithDatabase("kitchen"),
		pgcontainer.WithUsername("kitchen"),
		pgcontainer.WithPassword("kitchen"),
		pgcontainer.BasicWaitStrategies(),
	)
	if err != nil {
		return containers{}, fmt.Errorf("start postgres: %w", err)
	}

	redpanda, err := rpcontainer.Run(ctx, redpandaImage)
	if err != nil {
		_ = testcontainers.TerminateContainer(pg)

		return containers{}, fmt.Errorf("start redpanda: %w", err)
	}

	servers := containers{down: func() {
		_ = testcontainers.TerminateContainer(redpanda)
		_ = testcontainers.TerminateContainer(pg)
	}}

	if servers.dsn, err = pg.ConnectionString(ctx, "sslmode=disable"); err != nil {
		servers.down()

		return containers{}, fmt.Errorf("read the connection string: %w", err)
	}

	if servers.brokers, err = redpanda.KafkaSeedBroker(ctx); err != nil {
		servers.down()

		return containers{}, fmt.Errorf("read the seed broker: %w", err)
	}

	if err := migrate(ctx, servers.dsn); err != nil {
		servers.down()

		return containers{}, err
	}

	return servers, nil
}

// migrate applies the schema of the platform, file by file in the order the
// migration tool would.
func migrate(ctx context.Context, dsn string) error {
	files, err := filepath.Glob(filepath.Join("..", "..", "migrations", "kitchen", "*.up.sql"))
	if err != nil {
		return fmt.Errorf("list migrations: %w", err)
	}

	if len(files) == 0 {
		return errors.New("no migrations found next to the test")
	}

	sort.Strings(files)

	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return fmt.Errorf("connect for migrations: %w", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	for _, file := range files {
		statements, err := os.ReadFile(file) //nolint:gosec // the migrations of this repository
		if err != nil {
			return fmt.Errorf("read migration %s: %w", file, err)
		}

		if _, err := conn.Exec(ctx, string(statements)); err != nil {
			return fmt.Errorf("apply migration %s: %w", file, err)
		}
	}

	return nil
}

// fixture puts one venue with a menu of two dishes into the platform: the
// catalogue of the test is small on purpose, everything else is noise.
func fixture(ctx context.Context, dsn string) (uuid.UUID, map[string]uuid.UUID, error) {
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return uuid.Nil, nil, fmt.Errorf("connect for the fixture: %w", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	venueID, categoryID := uuid.New(), uuid.New()
	hash := sha256.Sum256([]byte(venueKey))

	const venueQuery = `
		INSERT INTO venues (id, slug, name, address, is_open, min_order_amount,
		                    delivery_fee, avg_cook_minutes)
		VALUES ($1, 'integration', 'Кофейня «Интеграция»', 'Москва, Тестовая ул., 1',
		        true, $2, $3, 10)`

	if _, err := conn.Exec(ctx, venueQuery, venueID, minimumOrder, deliveryFee); err != nil {
		return uuid.Nil, nil, fmt.Errorf("insert the venue: %w", err)
	}

	const keyQuery = `INSERT INTO venue_api_keys (venue_id, key_hash) VALUES ($1, $2)`

	if _, err := conn.Exec(ctx, keyQuery, venueID, hash[:]); err != nil {
		return uuid.Nil, nil, fmt.Errorf("insert the api key: %w", err)
	}

	const categoryQuery = `
		INSERT INTO menu_categories (id, venue_id, external_id, name, position)
		VALUES ($1, $2, 'CAT-ALL', 'Всё меню', 10)`

	if _, err := conn.Exec(ctx, categoryQuery, categoryID, venueID); err != nil {
		return uuid.Nil, nil, fmt.Errorf("insert the category: %w", err)
	}

	items, err := dishes(ctx, conn, venueID, categoryID)
	if err != nil {
		return uuid.Nil, nil, err
	}

	return venueID, items, nil
}

// dishes fills the menu of the fixture and returns the identifiers the
// customer API puts into a cart.
func dishes(
	ctx context.Context, conn *pgx.Conn, venueID, categoryID uuid.UUID,
) (map[string]uuid.UUID, error) {
	menu := []struct {
		externalID string
		name       string
		price      int64
		stock      *int
	}{
		{espresso, "Эспрессо", espressoPrice, nil},
		{cheesecake, "Чизкейк", cheesecakePrice, new(cheesecakeStock)},
	}

	const query = `
		INSERT INTO menu_items (id, venue_id, category_id, external_id, name, price, stock_qty)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`

	items := make(map[string]uuid.UUID, len(menu))

	for _, dish := range menu {
		id := uuid.New()

		_, err := conn.Exec(ctx, query,
			id, venueID, categoryID, dish.externalID, dish.name, dish.price, dish.stock)
		if err != nil {
			return nil, fmt.Errorf("insert dish %s: %w", dish.externalID, err)
		}

		items[dish.externalID] = id
	}

	return items, nil
}

// orderService is the use case the API, the jobs and the tests all move the
// orders through.
func orderService(pool *postgres.DB, topics config.Kafka) *orderusecase.Service {
	return orderusecase.New(
		pool,
		postgres.NewOrderRepo(pool),
		postgres.NewCartRepo(pool),
		postgres.NewMenuRepo(pool),
		postgres.NewOutboxRepo(pool),
		orderusecase.Topics{Orders: topics.OrdersTopic, Status: topics.StatusTopic},
	)
}

// mount registers both APIs of the platform the way the service does.
func mount(
	pool *postgres.DB,
	orders *orderusecase.Service,
	hub *sse.Hub,
	topics config.Kafka,
	log *slog.Logger,
) chi.Router {
	venues, menus := postgres.NewVenueRepo(pool), postgres.NewMenuRepo(pool)
	router := httpx.NewRouter(log)

	services := kitchen.Services{
		Catalog: catalogusecase.New(venues, menus),
		Cart:    cartusecase.New(pool, postgres.NewCartRepo(pool), menus),
		Order:   orders,
	}

	keys := kitchen.Idempotency{
		Store: postgres.NewIdempotencyRepo(pool),
		Tx:    pool,
		TTL:   time.Hour,
	}

	streams := kitchen.Streams{Hub: hub, Heartbeat: time.Second}

	if err := kitchen.Mount(router, services, keys, streams, log); err != nil {
		panic(err)
	}

	partner.Mount(router, partnerusecase.New(venues, menus), orders, topics.OrdersTopic, log)

	return router
}

// jobs are the background loops of the platform: what the outbox has collected
// goes to the broker, and an order long enough on its way is closed.
func jobs(
	pool *postgres.DB, orders *orderusecase.Service, producer *broker.Producer, log *slog.Logger,
) []worker.Job {
	return []worker.Job{
		worker.NewOutbox(pool, postgres.NewOutboxRepo(pool), producer, config.OutboxJob{
			BatchSize: 100,
			PollMin:   pollMin,
			PollMax:   pollMax,
		}, log),
		worker.NewCourier(orders, config.CourierJob{
			Delivery:  deliveryDuration,
			Interval:  jobInterval,
			BatchSize: 50,
		}, log),
	}
}

// follow reads the statuses of the orders into the hub the streams are served
// from, exactly as an instance of the API does.
func follow(ctx context.Context, topics config.Kafka, hub *sse.Hub, log *slog.Logger) {
	broker.Serve(ctx, broker.ConsumerConfig{
		Brokers: topics.Brokers,
		Topic:   topics.StatusTopic,
		GroupID: "sse-" + uuid.NewString(),
		// The topic is empty when the stand starts, so its beginning and its end
		// are the same place -- and reading from the beginning takes the joining
		// of the group out of the race with the first order of a test.
		FromLatest: false,
	}, consumer.NewStatus(hub, log).Handle, log)
}
