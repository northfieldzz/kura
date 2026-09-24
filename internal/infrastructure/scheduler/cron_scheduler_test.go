package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"
)

type mockBatchUseCase struct {
	monthlyReportCallCount int
	quotaAlertsCallCount   int
	monthlyReportErr       error
	quotaAlertsErr         error
}

func (m *mockBatchUseCase) RunMonthlyReport(ctx context.Context) error {
	m.monthlyReportCallCount++
	return m.monthlyReportErr
}

func (m *mockBatchUseCase) RunQuotaAlerts(ctx context.Context) error {
	m.quotaAlertsCallCount++
	return m.quotaAlertsErr
}

func (m *mockBatchUseCase) RunReconciliation(ctx context.Context) error {
	return nil
}

func TestCronScheduler_StartStop(t *testing.T) {
	mockUC := &mockBatchUseCase{}
	scheduler := NewCronScheduler(mockUC)

	scheduler.Start()
	time.Sleep(10 * time.Millisecond)
	scheduler.Stop()
}

func TestCronScheduler_Jobs(t *testing.T) {
	mockUC := &mockBatchUseCase{}
	scheduler := NewCronScheduler(mockUC)

	entries := scheduler.cron.Entries()
	if len(entries) != 2 {
		t.Fatalf("expected 2 cron jobs registered, got %d", len(entries))
	}

	for _, entry := range entries {
		entry.Job.Run()
	}

	if mockUC.monthlyReportCallCount != 1 {
		t.Errorf("expected RunMonthlyReport to be called 1 time, got %d", mockUC.monthlyReportCallCount)
	}

	if mockUC.quotaAlertsCallCount != 1 {
		t.Errorf("expected RunQuotaAlerts to be called 1 time, got %d", mockUC.quotaAlertsCallCount)
	}
}

func TestCronScheduler_Jobs_Error(t *testing.T) {
	mockUC := &mockBatchUseCase{
		monthlyReportErr: errors.New("monthly report error"),
		quotaAlertsErr:   errors.New("quota alerts error"),
	}
	scheduler := NewCronScheduler(mockUC)

	entries := scheduler.cron.Entries()
	if len(entries) != 2 {
		t.Fatalf("expected 2 cron jobs registered, got %d", len(entries))
	}

	for _, entry := range entries {
		entry.Job.Run()
	}

	if mockUC.monthlyReportCallCount != 1 {
		t.Errorf("expected RunMonthlyReport to be called 1 time, got %d", mockUC.monthlyReportCallCount)
	}

	if mockUC.quotaAlertsCallCount != 1 {
		t.Errorf("expected RunQuotaAlerts to be called 1 time, got %d", mockUC.quotaAlertsCallCount)
	}
}
