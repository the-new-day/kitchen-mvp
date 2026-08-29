// Command demo walks the whole scenario of the platform against a running
// compose: the catalogue, the cart, an order, its statuses in the event stream
// and every refusal the platform is required to answer with. It talks to the
// platform through the clients generated from the specifications, so a
// specification that has drifted from the service breaks the build of the demo.
package main

import (
	"avito-kitchen/internal/api/kitchenapi"
	"avito-kitchen/internal/api/partnerapi"
	"avito-kitchen/internal/config"
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
)

// settings is where the demo looks for the platform and the keys it acts with.
type settings struct {
	BaseURL        string        `env:"DEMO_BASE_URL"        envDefault:"http://localhost:8081/api/v1"`
	HealthURL      string        `env:"DEMO_HEALTH_URL"      envDefault:"http://localhost:8081/healthz"`
	BakeryKey      string        `env:"DEMO_BAKERY_API_KEY"  envDefault:"vk_demo_bakery_dev"`
	PizzeriaKey    string        `env:"DEMO_PIZZA_API_KEY"   envDefault:"vk_demo_pizza_dev"`
	RequestTimeout time.Duration `env:"DEMO_REQUEST_TIMEOUT" envDefault:"10s"`
	StartupWait    time.Duration `env:"DEMO_STARTUP_WAIT"    envDefault:"120s"`
	StatusWait     time.Duration `env:"DEMO_STATUS_WAIT"     envDefault:"120s"`
	PollInterval   time.Duration `env:"DEMO_POLL_INTERVAL"   envDefault:"1s"`
	AcceptTimeout  time.Duration `env:"ORDER_ACCEPT_TIMEOUT" envDefault:"30s"`
}

// step is one check of the scenario. The note it returns is what the demo
// tells about the check that passed.
type step struct {
	name string
	run  func(context.Context) (string, error)
}

// demo holds the clients of the scenario and what its steps hand over to each
// other: the venues it works with, the cart it has filled and the order it is
// following.
type demo struct {
	cfg      settings
	http     *http.Client
	streams  *http.Client
	eater    *customer
	bakery   *partnerapi.ClientWithResponses
	pizzeria *partnerapi.ClientWithResponses

	venue  kitchenapi.Venue
	menu   map[string]kitchenapi.MenuItem
	total  int64
	key    uuid.UUID
	order  kitchenapi.Order
	events *stream
}

func main() {
	os.Exit(run())
}

func run() int {
	outage := flag.Bool("outage", false,
		"проверить, что заказ доезжает до заведения после его простоя (останавливает venue-service)")
	flag.Parse()

	var cfg settings
	if err := config.ParseInto(&cfg); err != nil {
		fmt.Fprintf(os.Stderr, "demo: %v\n", err)

		return 1
	}

	d, err := newDemo(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "demo: %v\n", err)

		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := d.walk(ctx, d.steps(*outage)); err != nil {
		fmt.Printf("\nсценарий не прошёл: %v\n", err)

		return 1
	}

	fmt.Println("\nсценарий пройден целиком")

	return 0
}

func newDemo(cfg settings) (*demo, error) {
	client := &http.Client{Timeout: cfg.RequestTimeout}

	eater, err := newCustomer(cfg.BaseURL, client)
	if err != nil {
		return nil, err
	}

	bakery, err := newVenue(cfg.BaseURL, cfg.BakeryKey, client)
	if err != nil {
		return nil, err
	}

	pizzeria, err := newVenue(cfg.BaseURL, cfg.PizzeriaKey, client)
	if err != nil {
		return nil, err
	}

	return &demo{
		cfg:      cfg,
		http:     client,
		streams:  &http.Client{},
		eater:    eater,
		bakery:   bakery,
		pizzeria: pizzeria,
		menu:     map[string]kitchenapi.MenuItem{},
	}, nil
}

// steps is the scenario in the order the platform is meant to be used.
func (d *demo) steps(outage bool) []step {
	steps := []step{
		{"площадка отвечает", d.waitForPlatform},
		{"пекарня открыла смену и выгрузила меню", d.waitForBakery},
		{"корзина собрана", d.fillCart},
		{"корзина проверена, проблем нет", d.checkCart},
		{"цена изменилась — 409 price_mismatch", d.refuseOnPriceChange},
		{"заказ оформлен", d.placeOrder},
		{"повтор ключа отдаёт тот же заказ", d.repeatKey},
		{"ключ с другой корзиной — 422 idempotency_key_reuse", d.reuseKey},
		{"поток событий заказа открыт", d.openStream},
		{"заказ прошёл все статусы до DELIVERED", d.watchStatuses},
		{"позиция кончилась — 409 out_of_stock", d.refuseOnEmptyStock},
		{"заказ без accept отклонён системой", d.rejectOnTimeout},
	}

	if outage {
		steps = append(steps, step{"заказ доехал до заведения после простоя", d.surviveOutage})
	}

	return steps
}

// walk runs the scenario and stops at the first check that did not pass.
func (d *demo) walk(ctx context.Context, steps []step) error {
	for i, s := range steps {
		fmt.Printf("[%2d/%d] %s ... ", i+1, len(steps), s.name)

		note, err := s.run(ctx)
		if err != nil {
			fmt.Println("провал")

			return fmt.Errorf("%s: %w", s.name, err)
		}

		fmt.Println("ok")

		if note != "" {
			fmt.Printf("        %s\n", note)
		}
	}

	return nil
}

// until calls check every poll interval until it reports that it is done, the
// deadline runs out or ctx is cancelled. It is what the demo waits with: an
// event nobody promised a moment for is waited for, not slept through.
func (d *demo) until(
	ctx context.Context, wait time.Duration, check func(context.Context) (bool, error),
) error {
	deadline := time.NewTimer(wait)
	defer deadline.Stop()

	ticker := time.NewTicker(d.cfg.PollInterval)
	defer ticker.Stop()

	for {
		done, err := check(ctx)
		if err != nil {
			return err
		}

		if done {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("не дождались за %s", wait)
		case <-ticker.C:
		}
	}
}
