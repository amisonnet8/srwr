package timesheet

import (
	"reflect"
	"testing"
	"time"
)

func date(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func TestMonth(t *testing.T) {
	cases := []struct {
		year  int
		m     time.Month
		end   time.Time
		ndays int
	}{
		{2026, time.February, date(2026, 3, 1), 28},
		{2024, time.February, date(2024, 3, 1), 29},
		{2025, time.December, date(2026, 1, 1), 31},
	}
	for _, c := range cases {
		p := Month(c.year, c.m)
		if !p.Start.Equal(date(c.year, c.m, 1)) || !p.End.Equal(c.end) || p.Days() != c.ndays {
			t.Errorf("Month(%d, %v) = %v..%v (%d日)", c.year, c.m, p.Start, p.End, p.Days())
		}
	}
}

func TestContains(t *testing.T) {
	p := Month(2026, time.February)
	cases := []struct {
		t    time.Time
		want bool
	}{
		{date(2026, 1, 31), false},
		{date(2026, 2, 1), true},
		{date(2026, 2, 28), true},
		{time.Date(2026, 2, 28, 23, 59, 59, 0, time.UTC), true},
		{date(2026, 3, 1), false},
	}
	for _, c := range cases {
		if got := p.Contains(c.t); got != c.want {
			t.Errorf("Contains(%v) = %v, want %v", c.t, got, c.want)
		}
	}
}

func TestWeekStart(t *testing.T) {
	cases := []struct {
		in, want time.Time
	}{
		{date(2026, 2, 2), date(2026, 2, 2)},  // 月
		{date(2026, 2, 4), date(2026, 2, 2)},  // 水
		{date(2026, 2, 7), date(2026, 2, 2)},  // 土
		{date(2026, 2, 8), date(2026, 2, 2)},  // 日
		{date(2026, 2, 1), date(2026, 1, 26)}, // 日、前の月へ
		{time.Date(2026, 2, 3, 15, 30, 0, 0, time.UTC), date(2026, 2, 2)},
		{date(2026, 3, 4), date(2026, 3, 2)},
	}
	for _, c := range cases {
		if got := WeekStart(c.in); !got.Equal(c.want) {
			t.Errorf("WeekStart(%v) = %v, want %v", c.in.Format("2006-01-02 Mon"), got.Format("2006-01-02 Mon"), c.want.Format("2006-01-02 Mon"))
		}
	}
}

func TestWeeks(t *testing.T) {
	cases := []struct {
		name   string
		period Period
		starts []time.Time
	}{
		{"2026年2月（日曜始まりの月）", Month(2026, time.February),
			[]time.Time{date(2026, 2, 1), date(2026, 2, 2), date(2026, 2, 9), date(2026, 2, 16), date(2026, 2, 23)}},
		{"2026年4月（水曜始まりの月）", Month(2026, time.April),
			[]time.Time{date(2026, 4, 1), date(2026, 4, 6), date(2026, 4, 13), date(2026, 4, 20), date(2026, 4, 27)}},
		{"2026年3月", Month(2026, time.March),
			[]time.Time{date(2026, 3, 1), date(2026, 3, 2), date(2026, 3, 9), date(2026, 3, 16), date(2026, 3, 23), date(2026, 3, 30)}},
	}
	for _, c := range cases {
		weeks := c.period.Weeks()
		if len(weeks) != len(c.starts) {
			t.Errorf("%s: 週の数 = %d, want %d", c.name, len(weeks), len(c.starts))
			continue
		}
		days := 0
		for i, w := range weeks {
			if !w.Start.Equal(c.starts[i]) {
				t.Errorf("%s: %d週目の開始 = %v, want %v", c.name, i+1, w.Start.Format("01-02"), c.starts[i].Format("01-02"))
			}
			days += w.Days()
		}
		if last := weeks[len(weeks)-1]; !last.End.Equal(c.period.End) {
			t.Errorf("%s: 最後の週の終わり = %v", c.name, last.End)
		}
		if days != c.period.Days() {
			t.Errorf("%s: 週の日数の合計 = %d, want %d", c.name, days, c.period.Days())
		}
	}
}

func TestBusinessDays(t *testing.T) {
	holidays := map[string]bool{
		"2026-02-11": true, // 水
		"2026-02-14": true, // 土（すでに休み）
	}
	cases := []struct {
		name     string
		p        Period
		holidays map[string]bool
		want     int
	}{
		{"2026年2月", Month(2026, time.February), nil, 20},
		{"祝日あり", Month(2026, time.February), holidays, 19},
		{"2026年3月", Month(2026, time.March), nil, 22},
		{"日曜だけ", Period{date(2026, 2, 1), date(2026, 2, 2)}, nil, 0},
	}
	for _, c := range cases {
		if got := c.p.BusinessDays(c.holidays); got != c.want {
			t.Errorf("%s: BusinessDays = %d, want %d", c.name, got, c.want)
		}
	}
}

func TestAddValidation(t *testing.T) {
	cases := []struct {
		name    string
		r       Record
		wantErr bool
	}{
		{"正常", Record{date(2026, 2, 2), "alpha", 60}, false},
		{"上限ちょうど", Record{date(2026, 2, 2), "alpha", 1440}, false},
		{"上限超え", Record{date(2026, 2, 2), "alpha", 1441}, true},
		{"0分", Record{date(2026, 2, 2), "alpha", 0}, true},
		{"負", Record{date(2026, 2, 2), "alpha", -5}, true},
		{"プロジェクト名なし", Record{date(2026, 2, 2), "", 30}, true},
	}
	for _, c := range cases {
		var l Ledger
		if err := l.Add(c.r); (err != nil) != c.wantErr {
			t.Errorf("%s: err = %v, wantErr = %v", c.name, err, c.wantErr)
		}
	}
}

func TestAddNormalizesDate(t *testing.T) {
	var l Ledger
	_ = l.Add(Record{time.Date(2026, 2, 3, 17, 45, 0, 0, time.UTC), "alpha", 30})
	got := l.Between(Month(2026, time.February))
	if len(got) != 1 || !got[0].Date.Equal(date(2026, 2, 3)) {
		t.Errorf("records = %+v", got)
	}
}

func TestBetween(t *testing.T) {
	var l Ledger
	for _, r := range []Record{
		{date(2026, 3, 1), "alpha", 10},
		{date(2026, 2, 28), "beta", 20},
		{date(2026, 2, 10), "beta", 30},
		{date(2026, 2, 10), "alpha", 40},
		{date(2026, 2, 1), "alpha", 50},
		{date(2026, 1, 31), "alpha", 60},
	} {
		if err := l.Add(r); err != nil {
			t.Fatal(err)
		}
	}
	got := l.Between(Month(2026, time.February))
	want := []Record{
		{date(2026, 2, 1), "alpha", 50},
		{date(2026, 2, 10), "alpha", 40},
		{date(2026, 2, 10), "beta", 30},
		{date(2026, 2, 28), "beta", 20},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Between = %+v\nwant     %+v", got, want)
	}
	if n := len(l.Between(Period{date(2026, 4, 1), date(2026, 5, 1)})); n != 0 {
		t.Errorf("記録のない期間で %d 件返った", n)
	}
}

func TestProjects(t *testing.T) {
	var l Ledger
	if got := l.Projects(); len(got) != 0 {
		t.Errorf("空の台帳の Projects = %v", got)
	}
	_ = l.Add(Record{date(2026, 2, 2), "beta", 10})
	_ = l.Add(Record{date(2026, 2, 3), "alpha", 10})
	_ = l.Add(Record{date(2026, 2, 4), "beta", 10})
	if got, want := l.Projects(), []string{"alpha", "beta"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Projects = %v, want %v", got, want)
	}
}
