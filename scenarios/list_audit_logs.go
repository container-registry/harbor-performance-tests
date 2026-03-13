package scenarios

import (
	"context"
	"math"

	"github.com/goharbor/perf/pkg/config"
	"github.com/goharbor/perf/pkg/harbor"
	"github.com/goharbor/perf/pkg/runner"
)

type ListAuditLogs struct {
	cfg *config.Config
}

type listAuditLogsData struct {
	total int64
}

func (s *ListAuditLogs) Name() string { return "list-audit-logs" }

func (s *ListAuditLogs) Setup(ctx context.Context, h *harbor.Client) (runner.SharedData, error) {
	res, err := h.ListAuditLogs(ctx, 1, 1)
	if err != nil {
		return nil, err
	}
	return &listAuditLogsData{total: res.Total}, nil
}

func (s *ListAuditLogs) InitWorker(_ context.Context, _ *harbor.Client, _ runner.SharedData) (runner.WorkerState, error) {
	return nil, nil
}

func (s *ListAuditLogs) Run(ctx context.Context, h *harbor.Client, data runner.SharedData, _ runner.WorkerState) error {
	d := data.(*listAuditLogsData)
	pageSize := int64(15)
	pages := int64(math.Ceil(float64(d.total) / float64(pageSize)))
	page := int64(harbor.RandomIntBetween(1, int(pages)))
	_, err := h.ListAuditLogs(ctx, page, pageSize)
	return err
}

func (s *ListAuditLogs) Teardown(_ context.Context, _ *harbor.Client, _ runner.SharedData) error {
	return nil
}
