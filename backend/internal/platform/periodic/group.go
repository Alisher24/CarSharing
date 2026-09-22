package periodic

import (
	"context"
	"sync"
)

// RunAll joins every task before returning, so its caller can safely release shared dependencies.
func RunAll(ctx context.Context, tasks ...func(context.Context)) {
	var running sync.WaitGroup
	for _, task := range tasks {
		running.Go(func() { task(ctx) })
	}
	running.Wait()
}
