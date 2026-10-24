// Package quality holds the server's Go code to the limits core/codequality
// states, the ones the window's lint holds its TypeScript to.
package quality

import (
	"testing"

	"non24.app/core/codequality"
)

func TestTheModuleStaysWithinTheLimits(t *testing.T) {
	over, err := codequality.Over("../..")
	if err != nil {
		t.Fatal(err)
	}
	for _, function := range over {
		t.Errorf("%s (%s:%d) has complexity %d and %d lines; the limits are %d and %d",
			function.Name, function.File, function.Line, function.Complexity, function.Lines,
			codequality.MaxComplexity, codequality.MaxLines)
	}
}
