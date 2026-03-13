package scenarios

import (
	"context"
	"math"

	"github.com/goharbor/perf/pkg/config"
	"github.com/goharbor/perf/pkg/harbor"
	"github.com/goharbor/perf/pkg/runner"
)

type ListQuotas struct {
	cfg *config.Config
}

type listQuotasData struct {
	total int64
}

func (s *ListQuotas) Name() string { return "list-quotas" }

func (s *ListQuotas) Setup(ctx context.Context, h *harbor.Client) (runner.SharedData, error) {
	res, err := h.ListQuotas(ctx, 1, 1)
	if err != nil {
		return nil, err
	}
	return &listQuotasData{total: res.Total}, nil
}

func (s *ListQuotas) InitWorker(_ context.Context, _ *harbor.Client, _ runner.SharedData) (runner.WorkerState, error) {
	return nil, nil
}

func (s *ListQuotas) Run(ctx context.Context, h *harbor.Client, data runner.SharedData, _ runner.WorkerState) error {
	d := data.(*listQuotasData)
	pageSize := int64(15)
	pages := int64(math.Ceil(float64(d.total) / float64(pageSize)))
	page := int64(harbor.RandomIntBetween(1, int(pages)))
	_, err := h.ListQuotas(ctx, page, pageSize)
	return err
}

func (s *ListQuotas) Teardown(_ context.Context, _ *harbor.Client, _ runner.SharedData) error {
	return nil
}
