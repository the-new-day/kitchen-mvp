package kitchen

import (
	"avito-kitchen/internal/api/kitchenapi"
	"fmt"
	"net/http"
)

// idempotentExtension marks in the specification the operations that require
// an Idempotency-Key. Reading it from there is what keeps the middleware and
// the contract from drifting apart: adding a second such operation is one line
// in the specification and nothing here.
const idempotentExtension = "x-idempotent"

// idempotentOperations returns the routes of the customer API that require an
// Idempotency-Key, named as "METHOD /pattern" the way chi matched them.
func idempotentOperations() (map[string]struct{}, error) {
	spec, err := kitchenapi.GetSpec()
	if err != nil {
		return nil, fmt.Errorf("load customer api specification: %w", err)
	}

	methods := []string{
		http.MethodPost, http.MethodPut, http.MethodPatch,
		http.MethodDelete, http.MethodGet,
	}

	operations := make(map[string]struct{})

	for path, item := range spec.Paths.Map() {
		for _, method := range methods {
			operation := item.GetOperation(method)
			if operation == nil {
				continue
			}

			if operation.Extensions[idempotentExtension] == true {
				operations[method+" "+basePath+path] = struct{}{}
			}
		}
	}

	return operations, nil
}
