package metrics

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Collector records latency and success/failure counts for a scenario.
type Collector struct {
	mu        sync.Mutex
	latencies []time.Duration // all recorded latencies
	success   atomic.Int64
	failure   atomic.Int64
	startTime time.Time
}

// NewCollector creates a new metrics collector.
func NewCollector() *Collector {
	return &Collector{
		startTime: time.Now(),
	}
}

// Record records a single iteration's latency and success/failure.
func (c *Collector) Record(d time.Duration, ok bool) {
	c.mu.Lock()
	c.latencies = append(c.latencies, d)
	c.mu.Unlock()

	if ok {
		c.success.Add(1)
	} else {
		c.failure.Add(1)
	}
}

// Summary computes the summary metrics.
func (c *Collector) Summary() *SummaryResult {
	c.mu.Lock()
	sorted := make([]time.Duration, len(c.latencies))
	copy(sorted, c.latencies)
	c.mu.Unlock()

	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	n := len(sorted)
	if n == 0 {
		return &SummaryResult{}
	}

	elapsed := time.Since(c.startTime)
	total := c.success.Load() + c.failure.Load()

	return &SummaryResult{
		Avg:            avg(sorted),
		Min:            sorted[0],
		Med:            percentile(sorted, 50),
		Max:            sorted[n-1],
		P90:            percentile(sorted, 90),
		P95:            percentile(sorted, 95),
		SuccessRate:    float64(c.success.Load()) / float64(total),
		IterationsRate: float64(total) / elapsed.Seconds(),
		TotalSuccess:   c.success.Load(),
		TotalFailure:   c.failure.Load(),
		Duration:       elapsed,
	}
}

// SummaryResult holds computed metrics.
type SummaryResult struct {
	Avg            time.Duration
	Min            time.Duration
	Med            time.Duration
	Max            time.Duration
	P90            time.Duration
	P95            time.Duration
	SuccessRate    float64
	IterationsRate float64
	TotalSuccess   int64
	TotalFailure   int64
	Duration       time.Duration
}

// WriteSummaryJSON writes a backward-compatible summary JSON file.
func (s *SummaryResult) WriteSummaryJSON(outputDir, scenarioName string) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return err
	}

	summary := map[string]interface{}{
		"timestamp":      time.Now().UnixMilli(),
		"description":    scenarioName,
		"avg":            toHumanDuration(s.Avg),
		"min":            toHumanDuration(s.Min),
		"med":            toHumanDuration(s.Med),
		"max":            toHumanDuration(s.Max),
		"p90":            toHumanDuration(s.P90),
		"p95":            toHumanDuration(s.P95),
		"successRate":    toHumanRate(s.SuccessRate),
		"iterationsRate": fmt.Sprintf("%.2f/s", s.IterationsRate),
	}

	data, err := json.Marshal(summary)
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(outputDir, scenarioName+".summary.json"), data, 0644)
}

// WriteRunJSON writes the detailed run result JSON file.
func (s *SummaryResult) WriteRunJSON(outputDir, scenarioName string, runMeta *RunMeta) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return err
	}

	result := map[string]interface{}{
		"scenario":  scenarioName,
		"config":    runMeta,
		"startTime": runMeta.StartTime,
		"endTime":   time.Now(),
		"duration":  s.Duration.String(),
		"dataset": map[string]interface{}{
			"profile":     runMeta.Profile,
			"policy":      runMeta.DatasetPolicy,
			"fingerprint": runMeta.DatasetFingerprint,
		},
		"metrics": map[string]interface{}{
			"avg":            s.Avg.Milliseconds(),
			"min":            s.Min.Milliseconds(),
			"med":            s.Med.Milliseconds(),
			"max":            s.Max.Milliseconds(),
			"p90":            s.P90.Milliseconds(),
			"p95":            s.P95.Milliseconds(),
			"successRate":    s.SuccessRate,
			"iterationsRate": s.IterationsRate,
			"totalSuccess":   s.TotalSuccess,
			"totalFailure":   s.TotalFailure,
		},
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(outputDir, scenarioName+".run.json"), data, 0644)
}

// RunMeta holds metadata about a run for the detailed output.
type RunMeta struct {
	Workers            int       `json:"workers"`
	Iterations         int       `json:"iterations"`
	StartTime          time.Time `json:"startTime"`
	Profile            string    `json:"profile,omitempty"`
	DatasetPolicy      string    `json:"datasetPolicy,omitempty"`
	DatasetFingerprint string    `json:"datasetFingerprint,omitempty"`
}

// PrintSummary prints a human-readable summary to stdout.
func (s *SummaryResult) PrintSummary(name string) {
	total := s.TotalSuccess + s.TotalFailure
	fmt.Printf("\n  %-40s\n", name)
	fmt.Printf("    avg=%-10s min=%-10s med=%-10s max=%-10s\n",
		toHumanDuration(s.Avg), toHumanDuration(s.Min),
		toHumanDuration(s.Med), toHumanDuration(s.Max))
	fmt.Printf("    p(90)=%-10s p(95)=%-10s\n",
		toHumanDuration(s.P90), toHumanDuration(s.P95))
	fmt.Printf("    success=%s  iterations=%d  rate=%.2f/s\n",
		toHumanRate(s.SuccessRate), total, s.IterationsRate)
}

func avg(sorted []time.Duration) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	var sum time.Duration
	for _, d := range sorted {
		sum += d
	}
	return sum / time.Duration(len(sorted))
}

func percentile(sorted []time.Duration, pct float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Ceil(pct/100*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func toHumanDuration(d time.Duration) string {
	ms := float64(d) / float64(time.Millisecond)
	if ms < 1000 {
		ms = math.Floor(ms*100) / 100
		return fmt.Sprintf("%sms", trimFloat(ms))
	}
	s := math.Floor((ms/1000)*100) / 100
	return fmt.Sprintf("%ss", trimFloat(s))
}

func toHumanRate(rate float64) string {
	pct := math.Floor((rate*100)*100) / 100
	return fmt.Sprintf("%s%%", trimFloat(pct))
}

func trimFloat(v float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", v), "0"), ".")
}
