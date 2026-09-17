package entity

import (
	"testing"
	"time"
)

func TestFormatMonthJST_CutoffBoundary(t *testing.T) {
	// 8月31日 23:59:59.999999999 JST (UTC では 14:59:59.999999999)
	augustEndJST := time.Date(2026, 8, 31, 23, 59, 59, 999999999, JST)
	if month := FormatMonthJST(augustEndJST); month != "2026-08" {
		t.Errorf("expected 2026-08, got %s", month)
	}

	// UTC表現で渡しても JST 基準で 8月と判定されること
	augustEndUTC := augustEndJST.UTC()
	if month := FormatMonthJST(augustEndUTC); month != "2026-08" {
		t.Errorf("expected 2026-08 from UTC time, got %s", month)
	}

	// 9月1日 00:00:00.000000000 JST (UTC では 8月31日 15:00:00)
	septemberStartJST := time.Date(2026, 9, 1, 0, 0, 0, 0, JST)
	if month := FormatMonthJST(septemberStartJST); month != "2026-09" {
		t.Errorf("expected 2026-09, got %s", month)
	}

	// UTC表現 (8月31日 15:00:00 UTC) で渡しても、JST では 9月1日 00:00:00 なので 2026-09
	septemberStartUTC := septemberStartJST.UTC()
	if month := FormatMonthJST(septemberStartUTC); month != "2026-09" {
		t.Errorf("expected 2026-09 from UTC time, got %s", month)
	}
}
