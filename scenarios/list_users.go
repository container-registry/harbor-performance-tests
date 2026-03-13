package scenarios

import (
	"context"
	"math"

	"github.com/goharbor/perf/pkg/config"
	"github.com/goharbor/perf/pkg/harbor"
	"github.com/goharbor/perf/pkg/runner"
)

type ListUsers struct {
	cfg *config.Config
}

type listUsersData struct {
	total int64
}

func (s *ListUsers) Name() string { return "list-users" }

func (s *ListUsers) Setup(ctx context.Context, h *harbor.Client) (runner.SharedData, error) {
	res, err := h.ListUsers(ctx, 1, 1)
	if err != nil {
		return nil, err
	}
	return &listUsersData{total: res.Total}, nil
}

func (s *ListUsers) InitWorker(_ context.Context, _ *harbor.Client, _ runner.SharedData) (runner.WorkerState, error) {
	return nil, nil
}

func (s *ListUsers) Run(ctx context.Context, h *harbor.Client, data runner.SharedData, _ runner.WorkerState) error {
	d := data.(*listUsersData)
	pageSize := int64(15)
	pages := int64(math.Ceil(float64(d.total) / float64(pageSize)))
	page := int64(harbor.RandomIntBetween(1, int(pages)))
	_, err := h.ListUsers(ctx, page, pageSize)
	return err
}

func (s *ListUsers) Teardown(_ context.Context, _ *harbor.Client, _ runner.SharedData) error {
	return nil
}
