package main

import "fmt"

// INTENTIONAL: lint violations for e2e testing.
// Do NOT fix these — go-guardian's linter agent should detect them.

// Exported function without doc comment (golangci-lint: revive/exported).
func ProcessData(input string) string {
	// INTENTIONAL: error return value ignored (golangci-lint: errcheck).
	fmt.Println(input)

	// INTENTIONAL: unused variable (golangci-lint: ineffassign).
	result := "processed"
	result = "overwritten"
	_ = result

	return input
}
