package entity

import (
	"testing"
	"time"
)

func TestBuildNotificationPK(t *testing.T) {
	want := "NOTIFICATION#ALL"
	if got := BuildNotificationPK(); got != want {
		t.Errorf("BuildNotificationPK() = %v, want %v", got, want)
	}
}

func TestBuildNotificationSK(t *testing.T) {
	t.Run("UTC time", func(t *testing.T) {
		createdAt := time.Date(2023, 10, 1, 12, 0, 0, 123456000, time.UTC)
		id := "test-id-1"
		want := "8303838399876543999#test-id-1"
		if got := BuildNotificationSK(createdAt, id); got != want {
			t.Errorf("BuildNotificationSK() = %v, want %v", got, want)
		}
	})

	t.Run("Non-UTC time", func(t *testing.T) {
		jst := time.FixedZone("JST", 9*60*60) // UTC+9
		// 21:00 JST is 12:00 UTC
		createdAt := time.Date(2023, 10, 1, 21, 0, 0, 123456000, jst)
		id := "test-id-2"
		want := "8303838399876543999#test-id-2"
		if got := BuildNotificationSK(createdAt, id); got != want {
			t.Errorf("BuildNotificationSK() = %v, want %v", got, want)
		}
	})

	t.Run("No nanoseconds", func(t *testing.T) {
		createdAt := time.Date(2023, 10, 1, 12, 0, 0, 0, time.UTC)
		id := "test-id-3"
		want := "8303838399999999999#test-id-3"
		if got := BuildNotificationSK(createdAt, id); got != want {
			t.Errorf("BuildNotificationSK() = %v, want %v", got, want)
		}
	})
}
