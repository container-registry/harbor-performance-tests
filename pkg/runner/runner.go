package runner

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/goharbor/perf/pkg/harbor"
	"github.com/goharbor/perf/pkg/metrics"
	log "github.com/sirupsen/logrus"
)

// SharedData is opaque data returned by Setup and shared across all workers.
type SharedData = interface{}

// WorkerState is opaque per-worker state returned by InitWorker.
type WorkerState = interface{}

// Scenario defines the lifecycle of a single test scenario.
type Scenario interface {
	Name() string
	Setup(ctx context.Context, h *harbor.Client) (SharedData, error)
	InitWorker(ctx context.Context, h *harbor.Client, data SharedData) (WorkerState, error)
	Run(ctx context.Context, h *harbor.Client, data SharedData, state WorkerState) error
	Teardown(ctx context.Context, h *harbor.Client, data SharedData) error
}

// ClosedScheduler distributes a fixed number of iterations across workers.
type ClosedScheduler struct {
	Workers    int
	Iterations int
	Duration   time.Duration // optional timeout
}

// RunScenario executes a scenario using the closed-model scheduler.
func RunScenario(ctx context.Context, h *harbor.Client, scenario Scenario, sched ClosedScheduler) (*metrics.SummaryResult, error) {
	name := scenario.Name()
	fmt.Printf("\n▶ %s (workers=%d, iterations=%d)\n", name, sched.Workers, sched.Iterations)

	// Setup
	fmt.Printf("  setup...")
	data, err := scenario.Setup(ctx, h)
	if err != nil {
		return nil, fmt.Errorf("setup %s: %w", name, err)
	}
	fmt.Printf(" done\n")

	collector := metrics.NewCollector()
	var iterCounter atomic.Int64

	// Apply timeout if set
	runCtx := ctx
	var cancel context.CancelFunc
	if sched.Duration > 0 {
		runCtx, cancel = context.WithTimeout(ctx, sched.Duration)
		defer cancel()
	}

	// Launch workers
	var wg sync.WaitGroup
	for w := 0; w < sched.Workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			// InitWorker (per-goroutine)
			state, err := scenario.InitWorker(runCtx, h, data)
			if err != nil {
				log.Errorf("initWorker %s: %v", name, err)
				return
			}

			for {
				i := iterCounter.Add(1)
				if i > int64(sched.Iterations) {
					return
				}

				if runCtx.Err() != nil {
					return
				}

				start := time.Now()
				err := scenario.Run(runCtx, h, data, state)
				elapsed := time.Since(start)
				collector.Record(elapsed, err == nil)

				if err != nil {
					log.Debugf("%s iteration %d: %v", name, i, err)
				}
			}
		}()
	}

	wg.Wait()

	// Teardown
	fmt.Printf("  teardown...")
	if err := scenario.Teardown(ctx, h, data); err != nil {
		log.Warnf("teardown %s: %v", name, err)
	}
	fmt.Printf(" done\n")

	summary := collector.Summary()
	summary.PrintSummary(name)

	return summary, nil
}

// --- Scenario composition types (defined for future use) ---

// WeightedScenario pairs a scenario with a weight for mixed workloads.
type WeightedScenario struct {
	Scenario Scenario
	Weight   float64 // proportion of iterations (0.0-1.0)
}

// Stage defines a target worker count over a duration for ramping.
type Stage struct {
	Duration time.Duration
	Workers  int
}

// ThinkTime defines a pause range between iterations.
type ThinkTime struct {
	Min time.Duration
	Max time.Duration
}

// ScenarioGroup supports composition of multiple weighted scenarios.
type ScenarioGroup struct {
	Scenarios []WeightedScenario
	Stages    []Stage
	ThinkTime *ThinkTime
}
