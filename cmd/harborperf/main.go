package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/goharbor/perf/pkg/config"
	"github.com/goharbor/perf/pkg/harbor"
	"github.com/goharbor/perf/pkg/metrics"
	"github.com/goharbor/perf/pkg/prepare"
	"github.com/goharbor/perf/pkg/report"
	"github.com/goharbor/perf/pkg/runner"
	"github.com/goharbor/perf/scenarios"
	"github.com/spf13/cobra"
)

func main() {
	var apiOnly bool

	rootCmd := &cobra.Command{
		Use:   "harborperf",
		Short: "Harbor performance testing tool",
	}

	// list command
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List available test scenarios",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			for _, name := range scenarios.Names(cfg) {
				fmt.Println(name)
			}
			return nil
		},
	}

	// prepare command
	prepareCmd := &cobra.Command{
		Use:   "prepare",
		Short: "Prepare test data on the Harbor instance",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
			defer cancel()

			client, err := harbor.NewClient(&cfg.Harbor)
			if err != nil {
				return err
			}

			workers := cfg.VUs
			if workers > 300 {
				workers = 300
			}
			if workers < 10 {
				workers = 10
			}

			fmt.Printf("Preparing data (size=%s, policy=%s, workers=%d)\n",
				cfg.Size, cfg.DatasetPolicy, workers)
			return prepare.Execute(ctx, client, cfg, workers)
		},
	}

	// cleanup command
	cleanupCmd := &cobra.Command{
		Use:   "cleanup",
		Short: "Delete test projects and users",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
			defer cancel()

			client, err := harbor.NewClient(&cfg.Harbor)
			if err != nil {
				return err
			}

			workers := 50
			fmt.Println("Cleaning up test data...")
			return prepare.Cleanup(ctx, client, cfg, workers)
		},
	}

	// run command
	runCmd := &cobra.Command{
		Use:   "run [scenario...]",
		Short: "Run test scenarios",
		Long:  "Run one or more test scenarios. If no scenario names are given, runs all scenarios.",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
			defer cancel()

			client, err := harbor.NewClient(&cfg.Harbor)
			if err != nil {
				return err
			}

			if err := os.MkdirAll(cfg.OutputDir, 0755); err != nil {
				return err
			}

			var toRun []runner.Scenario
			if len(args) > 0 {
				for _, name := range args {
					s := scenarios.ByName(cfg, name)
					if s == nil {
						return fmt.Errorf("unknown scenario: %s", name)
					}
					toRun = append(toRun, s)
				}
			} else if apiOnly {
				toRun = scenarios.APIOnly(cfg)
			} else {
				toRun = scenarios.All(cfg)
			}

			sched := runner.ClosedScheduler{
				Workers:    cfg.VUs,
				Iterations: cfg.Iterations,
			}

			startTime := time.Now()

			fmt.Printf("Running %d scenarios (vus=%d, iterations=%d)\n",
				len(toRun), sched.Workers, sched.Iterations)

			for _, s := range toRun {
				summary, err := runner.RunScenario(ctx, client, s, sched)
				if err != nil {
					fmt.Fprintf(os.Stderr, "ERROR %s: %v\n", s.Name(), err)
					continue
				}

				if err := summary.WriteSummaryJSON(cfg.OutputDir, s.Name()); err != nil {
					fmt.Fprintf(os.Stderr, "write summary %s: %v\n", s.Name(), err)
				}

				runMeta := &metrics.RunMeta{
					Workers:    sched.Workers,
					Iterations: sched.Iterations,
					StartTime:  startTime,
				}
				if err := summary.WriteRunJSON(cfg.OutputDir, s.Name(), runMeta); err != nil {
					fmt.Fprintf(os.Stderr, "write run result %s: %v\n", s.Name(), err)
				}
			}

			if cfg.ReportEnabled {
				return report.MarkdownReport()
			}

			return nil
		},
	}
	runCmd.Flags().BoolVar(&apiOnly, "api-only", false, "Run only API tests (no push/pull)")

	// compare command
	compareCmd := &cobra.Command{
		Use:   "compare <dir1> <dir2>",
		Short: "Compare two test result directories",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return report.Compare(args[0], args[1])
		},
	}

	rootCmd.AddCommand(listCmd, prepareCmd, cleanupCmd, runCmd, compareCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
