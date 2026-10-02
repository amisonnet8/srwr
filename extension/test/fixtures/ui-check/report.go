package timesheet

import (
	"sort"
	"time"
)

const (
	billingUnit   = 15     // 請求は15分単位
	dailyStandard = 8 * 60 // 1営業日の所定時間（分）
)

// roundBilling は分を請求単位に切り上げる。
func roundBilling(mins int) int {
	return (mins + billingUnit - 1) / billingUnit * billingUnit
}

// ProjectTotal はプロジェクトごとの月次集計。
type ProjectTotal struct {
	Project  string
	Minutes  int // 実作業時間
	Billable int // 請求時間。プロジェクトごと・日ごとに切り上げて合計する
}

// MonthlyReport は year 年 m 月のプロジェクト別集計をプロジェクト名順で返す。
func MonthlyReport(l *Ledger, year int, m time.Month) []ProjectTotal {
	type key struct {
		project string
		day     string
	}
	perDay := map[key]int{}
	raw := map[string]int{}
	for _, r := range l.Between(Month(year, m)) {
		perDay[key{r.Project, r.Date.Format("2006-01-02")}] += r.Minutes
		raw[r.Project] += r.Minutes
	}
	billable := map[string]int{}
	for k, mins := range perDay {
		billable[k.project] += roundBilling(mins)
	}
	var out []ProjectTotal
	for p, mins := range raw {
		out = append(out, ProjectTotal{Project: p, Minutes: mins, Billable: billable[p]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Project < out[j].Project })
	return out
}

// WeekSummary は1週間ぶんの作業時間と残業時間。
type WeekSummary struct {
	Week     Period
	Minutes  int
	Overtime int // 所定時間（営業日数 x 8時間）を超えた分
}

// WeeklySummaries は期間を週ごとに区切って合計する。
func WeeklySummaries(l *Ledger, p Period, holidays map[string]bool) []WeekSummary {
	var out []WeekSummary
	for _, w := range p.Weeks() {
		total := 0
		for _, r := range l.Between(w) {
			total += r.Minutes
		}
		limit := w.BusinessDays(holidays) * dailyStandard
		over := total - limit
		if over < 0 {
			over = 0
		}
		out = append(out, WeekSummary{Week: w, Minutes: total, Overtime: over})
	}
	return out
}
