package entity

import "time"

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

// BuildNotificationPK は通知レコードの Partition Key を生成する
func BuildNotificationPK() string {
	return "NOTIFICATIONS"
}

// BuildNotificationSK は通知レコードの Sort Key を生成する (RFC3339Nano で時系列降順ソート可能)
func BuildNotificationSK(createdAt time.Time, id string) string {
	return "NOTIFICATION#" + createdAt.UTC().Format(time.RFC3339Nano) + "#" + id
}
