package timesheet

import "time"

// Period は [Start, End) の半開区間。すべて UTC の 0 時で扱う。
type Period struct {
	Start time.Time
	End   time.Time
}

// Month は year 年 m 月の期間を返す。
func Month(year int, m time.Month) Period {
	s := time.Date(year, m, 1, 0, 0, 0, 0, time.UTC)
	return Period{Start: s, End: s.AddDate(0, 1, 0)}
}

// Contains は t が期間に含まれるかを返す。
func (p Period) Contains(t time.Time) bool {
	return !t.Before(p.Start) && t.Before(p.End)
}

// Days は期間の日数。
func (p Period) Days() int {
	return int(p.End.Sub(p.Start) / (24 * time.Hour))
}

// WeekStart は t を含む週の月曜 0 時を返す（週は月曜始まり）。
func WeekStart(t time.Time) time.Time {
	d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	offset := (int(d.Weekday()) - int(time.Monday) + 7) % 7
	return d.AddDate(0, 0, -offset)
}

// Weeks は期間を月曜始まりの週ごとに分ける。
// 最初と最後の週は期間の端で切り詰められる。
func (p Period) Weeks() []Period {
	var out []Period
	cur := p.Start
	for cur.Before(p.End) {
		next := WeekStart(cur).AddDate(0, 0, 7)
		if next.After(p.End) {
			next = p.End
		}
		out = append(out, Period{Start: cur, End: next})
		cur = next
	}
	return out
}

// BusinessDays は期間内の平日（月〜金）の日数。holidays に含まれる日は除く。
// holidays のキーは "2006-01-02" 形式。
func (p Period) BusinessDays(holidays map[string]bool) int {
	n := 0
	for d := p.Start; d.Before(p.End); d = d.AddDate(0, 0, 1) {
		if wd := d.Weekday(); wd == time.Saturday || wd == time.Sunday {
			continue
		}
		if holidays[d.Format("2006-01-02")] {
			continue
		}
		n++
	}
	return n
}
