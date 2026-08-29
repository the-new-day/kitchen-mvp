package integration_test

import (
	"context"
	"flag"
	"fmt"
	"os"
	"testing"
	"time"
)

// startTimeout is how long the containers and the schema are given to come up.
// The first run of the test on a machine pulls the images, which is most of it.
const startTimeout = 10 * time.Minute

// live is the stand every test of this package runs against: the containers
// are brought up once, and the tests share what is behind the HTTP API.
var live *stand

func TestMain(m *testing.M) {
	flag.Parse()

	if testing.Short() {
		os.Exit(m.Run())
	}

	ctx, cancel := context.WithTimeout(context.Background(), startTimeout)
	defer cancel()

	built, err := start(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration: %v\n", err)

		os.Exit(1)
	}

	live = built
	code := m.Run()

	live.stop()
	os.Exit(code)
}

// platform returns the running stand, skipping the test when the run asked for
// the short tests only: the containers need a Docker to start in.
func platform(t *testing.T) *stand {
	t.Helper()

	if testing.Short() {
		t.Skip("the integration test needs Docker and is skipped in the short run")
	}

	return live
}
