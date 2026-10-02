package timesheet

import (
	"reflect"
	"testing"
	"time"
)

func sampleLedger(t *testing.T) *Ledger {
	t.Helper()
	l := &Ledger{}
	add := func(d time.Time, project string, mins int) {
		if err := l.Add(Record{d, project, mins}); err != nil {
			t.Fatal(err)
		}
	}
	add(date(2026, 1, 31), "beta", 200)
	add(date(2026, 2, 1), "alpha", 60)
	add(date(2026, 2, 2), "alpha", 100)
	add(date(2026, 2, 2), "alpha", 20)
	add(date(2026, 2, 2), "beta", 30)
	add(date(2026, 2, 3), "alpha", 50)
	add(date(2026, 2, 4), "beta", 500)
	for d := 9; d <= 13; d++ {
		add(date(2026, 2, d), "alpha", 600)
	}
	add(date(2026, 2, 27), "beta", 45)
	add(date(2026, 2, 28), "alpha", 90)
	add(date(2026, 3, 1), "alpha", 300)
	return l
}

func TestMonthlyReport(t *testing.T) {
	got := MonthlyReport(sampleLedger(t), 2026, time.February)
	want := []ProjectTotal{
		{"alpha", 3320, 3330},
		{"beta", 575, 585},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("MonthlyReport = %+v\nwant           %+v", got, want)
	}
	if got := MonthlyReport(sampleLedger(t), 2026, time.April); len(got) != 0 {
		t.Errorf("記録のない月のレポート = %+v", got)
	}
}

func TestWeeklySummaries(t *testing.T) {
	feb := Month(2026, time.February)
	got := WeeklySummaries(sampleLedger(t), feb, nil)
	want := []WeekSummary{
		{Period{date(2026, 2, 1), date(2026, 2, 2)}, 60, 60},
		{Period{date(2026, 2, 2), date(2026, 2, 9)}, 700, 0},
		{Period{date(2026, 2, 9), date(2026, 2, 16)}, 3000, 600},
		{Period{date(2026, 2, 16), date(2026, 2, 23)}, 0, 0},
		{Period{date(2026, 2, 23), date(2026, 3, 1)}, 135, 0},
	}
	if !reflect.DeepEqual(got, want) {
		for i := range got {
			t.Logf("got[%d] = %+v", i, got[i])
		}
		t.Errorf("WeeklySummaries が期待と違う（%d 週, want %d 週）", len(got), len(want))
	}

	holidays := map[string]bool{"2026-02-11": true}
	got = WeeklySummaries(sampleLedger(t), feb, holidays)
	if len(got) != 5 {
		t.Fatalf("週の数 = %d, want 5", len(got))
	}
	if got[2].Overtime != 1080 {
		t.Errorf("祝日のある週の残業 = %d, want 1080", got[2].Overtime)
	}
}
