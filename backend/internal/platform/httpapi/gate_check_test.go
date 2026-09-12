package httpapi

import "testing"

// TestRequiredPipelineFailsOnABrokenTest is a temporary probe proving the required pipeline
// actually blocks a merge. It is reverted in the next commit.
func TestRequiredPipelineFailsOnABrokenTest(t *testing.T) {
	t.Fatal("deliberately broken test proving the required pipeline blocks a merge")
}
