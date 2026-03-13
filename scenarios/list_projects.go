package scenarios

import (
	"context"
	"math"

	"github.com/goharbor/perf/pkg/config"
	"github.com/goharbor/perf/pkg/harbor"
	"github.com/goharbor/perf/pkg/runner"
)

type ListProjects struct {
	cfg *config.Config
}

type listProjectsData struct {
	total int64
}

func (s *ListProjects) Name() string { return "list-projects" }

func (s *ListProjects) Setup(ctx context.Context, h *harbor.Client) (runner.SharedData, error) {
	res, err := h.ListProjects(ctx, 1, 1)
	if err != nil {
		return nil, err
	}
	return &listProjectsData{total: res.Total}, nil
}

func (s *ListProjects) InitWorker(_ context.Context, _ *harbor.Client, _ runner.SharedData) (runner.WorkerState, error) {
	return nil, nil
}

func (s *ListProjects) Run(ctx context.Context, h *harbor.Client, data runner.SharedData, _ runner.WorkerState) error {
	d := data.(*listProjectsData)
	pageSize := int64(15)
	pages := int64(math.Ceil(float64(d.total) / float64(pageSize)))
	page := int64(harbor.RandomIntBetween(1, int(pages)))

	_, err := h.ListProjects(ctx, page, pageSize)
	return err
}

func (s *ListProjects) Teardown(_ context.Context, _ *harbor.Client, _ runner.SharedData) error {
	return nil
}
