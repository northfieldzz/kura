package entity

import (
	"fmt"
	"time"
)

// NotificationType は通知の種別 (alert: 予算警告 / report: 月次集計レポート等)
type NotificationType string

const (
	NotificationTypeAlert  NotificationType = "alert"
	NotificationTypeReport NotificationType = "report"
	NotificationTypeInfo   NotificationType = "info"
)

// Notification はシステム内通知・アラートレコード
// PK: NOTIFICATIONS, SK: NOTIFICATION#<timestamp>#<id>
type Notification struct {
	PK        string           `json:"pk" dynamodbav:"pk"`
	SK        string           `json:"sk" dynamodbav:"sk"`
	ID        string           `json:"id" dynamodbav:"id"`
	Type      NotificationType `json:"type" dynamodbav:"type"`
	Title     string           `json:"title" dynamodbav:"title"`
	Message   string           `json:"message" dynamodbav:"message"`
	IsAlert   bool             `json:"is_alert" dynamodbav:"is_alert"`
	CreatedAt time.Time        `json:"created_at" dynamodbav:"created_at"`
}

// BuildNotificationPK は通知のパーティションキーを生成する
func BuildNotificationPK() string {
	return "NOTIFICATION#ALL"
}

// BuildNotificationSK は通知のソートキーを生成する（降順ソート用に反転時刻）
func BuildNotificationSK(createdAt time.Time, id string) string {
	// 最新が先頭に来るように 9999999999999999999 から UnixNano を引く
	inverted := uint64(9999999999999999999) - uint64(createdAt.UnixNano())
	return fmt.Sprintf("%019d#%s", inverted, id)
}
