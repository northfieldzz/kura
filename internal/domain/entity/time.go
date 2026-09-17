package entity

import "time"

// JST は日本標準時 (Asia/Tokyo, UTC+9) のタイムゾーン
var JST = time.FixedZone("Asia/Tokyo", 9*60*60)

// CurrentMonthJST は現在の日時を日本時間 (JST) 基準で "YYYY-MM" 形式の文字列として返す。
// 日本時間の月末日 23:59:59.999999999 までは当月キー、翌日 00:00:00.000 になった瞬間に翌月キーへと切り替わる。
func CurrentMonthJST() string {
	return FormatMonthJST(time.Now())
}

// FormatMonthJST は指定された日時を日本時間 (JST) 基準で "YYYY-MM" 形式の文字列としてフォーマットする。
func FormatMonthJST(t time.Time) string {
	return t.In(JST).Format("2006-01")
}
