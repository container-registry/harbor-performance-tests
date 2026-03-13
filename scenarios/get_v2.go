package scenarios

import (
	"context"
	"fmt"

	"github.com/goharbor/perf/pkg/config"
	"github.com/goharbor/perf/pkg/harbor"
	"github.com/goharbor/perf/pkg/runner"
)

type GetV2 struct {
	cfg *config.Config
}

func (s *GetV2) Name() string { return "get-v2" }

func (s *GetV2) Setup(_ context.Context, _ *harbor.Client) (runner.SharedData, error) {
	return nil, nil
}

func (s *GetV2) InitWorker(_ context.Context, _ *harbor.Client, _ runner.SharedData) (runner.WorkerState, error) {
	return nil, nil
}

func (s *GetV2) Run(ctx context.Context, h *harbor.Client, _ runner.SharedData, _ runner.WorkerState) error {
	status, err := h.GetV2(ctx)
	if err != nil {
		return err
	}
	if status != 200 && status != 401 {
		return fmt.Errorf("unexpected http status code: %d", status)
	}
	return nil
}

func (s *GetV2) Teardown(_ context.Context, _ *harbor.Client, _ runner.SharedData) error {
	return nil
}
