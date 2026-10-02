package timesheet

import (
	"fmt"
	"sort"
	"time"
)

const maxMinutesPerDay = 24 * 60

// Record は1件の作業記録。Date は日付（時刻は無視される）。
type Record struct {
	Date    time.Time
	Project string
	Minutes int
}

// Ledger は作業記録の集まり。
type Ledger struct {
	records []Record
}

// Add は記録を追加する。日付は UTC の 0 時にそろえる。
func (l *Ledger) Add(r Record) error {
	if r.Project == "" {
		return fmt.Errorf("プロジェクト名が空です")
	}
	if r.Minutes <= 0 || r.Minutes > maxMinutesPerDay {
		return fmt.Errorf("作業時間が範囲外です: %d分", r.Minutes)
	}
	r.Date = time.Date(r.Date.Year(), r.Date.Month(), r.Date.Day(), 0, 0, 0, 0, time.UTC)
	l.records = append(l.records, r)
	return nil
}

// Between は期間に含まれる記録を、日付、プロジェクト名の順に並べて返す。
func (l *Ledger) Between(p Period) []Record {
	var out []Record
	for _, r := range l.records {
		if p.Contains(r.Date) {
			out = append(out, r)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].Date.Equal(out[j].Date) {
			return out[i].Date.Before(out[j].Date)
		}
		return out[i].Project < out[j].Project
	})
	return out
}

// Projects は記録に現れるプロジェクト名を昇順で返す。
func (l *Ledger) Projects() []string {
	seen := map[string]bool{}
	var out []string
	for _, r := range l.records {
		if !seen[r.Project] {
			seen[r.Project] = true
			out = append(out, r.Project)
		}
	}
	sort.Strings(out)
	return out
}
