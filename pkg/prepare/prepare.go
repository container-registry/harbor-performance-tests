package prepare

import (
	"context"
	"fmt"

	"github.com/goharbor/perf/pkg/config"
	"github.com/goharbor/perf/pkg/harbor"
)

// PrepStep defines a single data preparation step.
type PrepStep interface {
	Name() string
	Verify(ctx context.Context, h *harbor.Client, cfg *config.Config) error
	Run(ctx context.Context, h *harbor.Client, cfg *config.Config, workers int) error
	Clean(ctx context.Context, h *harbor.Client, cfg *config.Config, workers int) error
}

// Steps returns the ordered preparation steps.
func Steps() []PrepStep {
	return []PrepStep{
		&ProjectStep{},
		&UserStep{},
		&MemberStep{},
		&ArtifactStep{},
		&TagStep{},
		&AuditLogStep{},
		&VulnerabilityStep{},
	}
}

// Execute runs all preparation steps according to the dataset policy.
func Execute(ctx context.Context, h *harbor.Client, cfg *config.Config, workers int) error {
	steps := Steps()
	policy := cfg.DatasetPolicy

	for _, step := range steps {
		fmt.Printf("  [%s] %s...\n", policy, step.Name())

		switch policy {
		case config.PolicyFresh:
			if err := step.Clean(ctx, h, cfg, workers); err != nil {
				fmt.Printf("    clean warning: %v\n", err)
			}
			if err := step.Run(ctx, h, cfg, workers); err != nil {
				return fmt.Errorf("step %s run: %w", step.Name(), err)
			}

		case config.PolicyVerify:
			if err := step.Verify(ctx, h, cfg); err != nil {
				return fmt.Errorf("step %s verify failed: %w", step.Name(), err)
			}

		case config.PolicyReuse:
			if err := step.Verify(ctx, h, cfg); err != nil {
				fmt.Printf("    verify failed, running: %v\n", err)
				if err := step.Run(ctx, h, cfg, workers); err != nil {
					return fmt.Errorf("step %s run: %w", step.Name(), err)
				}
			} else {
				fmt.Printf("    already exists, skipping\n")
			}
		}
	}

	return nil
}

// Cleanup runs the clean step for all preparation steps in reverse order.
func Cleanup(ctx context.Context, h *harbor.Client, cfg *config.Config, workers int) error {
	steps := Steps()
	// Clean in reverse order
	for i := len(steps) - 1; i >= 0; i-- {
		step := steps[i]
		fmt.Printf("  cleaning %s...\n", step.Name())
		if err := step.Clean(ctx, h, cfg, workers); err != nil {
			fmt.Printf("    warning: %v\n", err)
		}
	}
	return nil
}
