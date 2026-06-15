// Command validatecontracts validates the versioned JSON Schemas and the
// synthetic fixtures against them. It replaces the former check-jsonschema
// (Python) invocation in scripts/validate-contracts.*.
package main

import (
	"fmt"
	"os"

	"non24.app/tools/internal/repo"
	"non24.app/tools/internal/schema"
)

func main() {
	root, err := repo.Root()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := schema.ValidateAll(root); err != nil {
		fmt.Fprintln(os.Stderr, "contract validation failed:")
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("Validated all v1 fixtures and sample configuration.")
}
