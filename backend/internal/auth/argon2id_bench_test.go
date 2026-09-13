package auth

import (
	"runtime"
	"testing"
)

// deployedParameters are the parameters the service ships with, restated here so the benchmark
// measures what is actually deployed rather than a convenient smaller cost.
var deployedParameters = hashingParameters{
	MemoryKiB: 19 * 1024, Passes: 2, Parallelism: 1, SaltLength: SaltLength, KeyLength: KeyLength,
}

// BenchmarkDeployedHashing measures one password hash at the deployed cost. It is run in the target
// Docker environment and its result is recorded in the README beside the settings, so that the
// documented cost and the shipped cost cannot drift apart unnoticed.
func BenchmarkDeployedHashing(b *testing.B) {
	hasher := newPasswordHasher(deployedParameters, 1)
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	for i := 0; i < b.N; i++ {
		if _, err := hasher.Hash("correcthorsebattery"); err != nil {
			b.Fatal(err)
		}
	}
	runtime.ReadMemStats(&after)
	allocatedPerHash := (after.TotalAlloc - before.TotalAlloc) / uint64(max(b.N, 1))
	b.ReportMetric(float64(allocatedPerHash)/(1<<20), "MiB/hash")
}
