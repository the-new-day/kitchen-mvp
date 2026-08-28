package kitchenapi_test

import (
	"avito-kitchen/internal/api/kitchenapi"
	"testing"
)

// idempotentExtension marks the operations that require an Idempotency-Key.
// The middleware reads it from the embedded specification instead of keeping
// its own list of routes.
const idempotentExtension = "x-idempotent"

func TestIdempotentOperationsAreMarkedInTheSpec(t *testing.T) {
	t.Parallel()

	spec, err := kitchenapi.GetSpec()
	if err != nil {
		t.Fatalf("load embedded spec: %v", err)
	}

	cases := map[string]struct {
		path       string
		method     string
		idempotent bool
	}{
		"create order":  {path: "/orders", method: "POST", idempotent: true},
		"list orders":   {path: "/orders", method: "GET"},
		"cancel order":  {path: "/orders/{order_id}/cancel", method: "POST"},
		"put cart item": {path: "/cart/items", method: "PUT"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			item := spec.Paths.Find(tc.path)
			if item == nil {
				t.Fatalf("path %s is missing from the spec", tc.path)
			}

			op := item.GetOperation(tc.method)
			if op == nil {
				t.Fatalf("%s %s is missing from the spec", tc.method, tc.path)
			}

			value, ok := op.Extensions[idempotentExtension]
			if ok != tc.idempotent {
				t.Fatalf("%s %s: %s present = %v, want %v",
					tc.method, tc.path, idempotentExtension, ok, tc.idempotent)
			}
			if tc.idempotent && value != true {
				t.Fatalf("%s %s: %s = %v, want true",
					tc.method, tc.path, idempotentExtension, value)
			}
		})
	}
}
