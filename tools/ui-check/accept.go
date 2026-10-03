package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/amisonnet8/srwr/internal/uicheck"
)

// acceptResult records what the person decided about the last run (ui-check-result/latest). OK makes the screens of the run
// the new baseline, for every capture that differs or is new; NG keeps the baseline and records the note.
func acceptResult(root string, ok bool, note string, out io.Writer) error {
	latest := filepath.Join(root, "ui-check-result", "latest")
	data, err := os.ReadFile(filepath.Join(latest, "result.json")) //nolint:gosec // our own result directory
	if err != nil {
		return fmt.Errorf("no result yet: run qsoku ui-check first (%w)", err)
	}
	var rep Report
	if err := json.Unmarshal(data, &rep); err != nil {
		return err
	}
	if rep.Decision.State != "pending" {
		return fmt.Errorf("this result was already decided (%s at %s); run qsoku ui-check again", rep.Decision.State, rep.Decision.At)
	}
	var replaced []string
	if ok {
		for _, c := range rep.Checks {
			if !c.OK {
				return fmt.Errorf("the check %q failed; fix it before accepting (the baseline is not changed)", c.Name)
			}
		}
		if len(rep.Problems) > 0 {
			return errors.New("the run found problems (see the page); fix them before accepting (the baseline is not changed)")
		}
		for _, c := range append(append([]CaptureResult{}, rep.Vim...), rep.VSCode...) {
			if c.Status == statusError {
				return fmt.Errorf("%s could not be captured; the baseline is not changed", c.Name)
			}
		}
		for _, c := range rep.Vim {
			if c.Status == statusSame {
				continue
			}
			if err := replaceVimBaseline(root, latest, c); err != nil {
				return err
			}
			replaced = append(replaced, c.Name)
		}
		for _, c := range rep.VSCode {
			if c.Status == statusSame {
				continue
			}
			b, err := os.ReadFile(filepath.Join(latest, c.File)) //nolint:gosec // our own result directory
			if err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(root, c.Target), b, 0o600); err != nil { //nolint:gosec // the baseline of the extension
				return err
			}
			replaced = append(replaced, "vscode "+c.Name)
		}
	}
	rep.Decision = Decision{State: "ng", At: time.Now().Format(time.RFC3339), Note: note}
	if ok {
		rep.Decision.State = "ok"
	}
	b, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	for _, dir := range []string{latest, filepath.Join(root, "ui-check-result", rep.Dir)} {
		if _, err := os.Stat(dir); err == nil {
			if err := os.WriteFile(filepath.Join(dir, "result.json"), append(b, '\n'), 0o600); err != nil {
				return err
			}
		}
	}
	if len(replaced) > 0 {
		if err := noteBaselineChange(root, rep.Version, replaced); err != nil {
			return err
		}
	}
	switch {
	case !ok:
		_, _ = fmt.Fprintf(out, "NG を記録しました（基準は替えていません）：%s\n", note)
	case len(replaced) == 0:
		_, _ = fmt.Fprintln(out, "OK を記録しました。違う画面・新しい画面は無く、基準はそのままです。")
	default:
		_, _ = fmt.Fprintf(out, "OK を記録し、基準を差し替えました（%d）：%s\n基準はコミットしてください（vim/test/baseline、extension/test/baseline）。\n", len(replaced), strings.Join(replaced, ", "))
	}
	return nil
}

// replaceVimBaseline writes the screens of a capture as the baseline. The rows a baseline does not compare (Skip: each has its
// reason in vim/test/baseline/README.md) are kept for the frames that were there.
func replaceVimBaseline(root, latest string, c CaptureResult) error {
	data, err := os.ReadFile(filepath.Join(latest, c.File)) //nolint:gosec // our own result directory
	if err != nil {
		return err
	}
	now, err := uicheck.ReadBaseline(data)
	if err != nil {
		return err
	}
	if old, err := os.ReadFile(filepath.Join(root, c.Target)); err == nil { //nolint:gosec // the baseline of the Vim client
		if want, err := uicheck.ReadBaseline(old); err == nil {
			for i := range now.Frames {
				if i < len(want.Frames) {
					now.Frames[i].Skip = want.Frames[i].Skip
				}
			}
		}
	}
	out, err := uicheck.MarshalBaseline(now)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, c.Target), out, 0o600) //nolint:gosec // the baseline of the Vim client
}

// noteBaselineChange adds a line to the README of the baselines that changed.
func noteBaselineChange(root, version string, names []string) error {
	line := fmt.Sprintf("\n- %s `qsoku ui-accept`（版 %s）：%s を、人間が OK を付けた画面に差し替えた\n", time.Now().Format("2006-01-02"), version, strings.Join(names, "、"))
	for _, readme := range []string{filepath.Join("vim", "test", "baseline", "README.md"), filepath.Join("extension", "test", "baseline", "README.md")} {
		var touched bool
		for _, n := range names {
			if strings.HasPrefix(n, "vscode ") == strings.HasPrefix(readme, "extension") {
				touched = true
			}
		}
		if !touched {
			continue
		}
		f, err := os.OpenFile(filepath.Join(root, readme), os.O_APPEND|os.O_WRONLY, 0o600) //nolint:gosec // the README next to the baseline
		if err != nil {
			return err
		}
		_, werr := f.WriteString(line)
		if cerr := f.Close(); werr == nil {
			werr = cerr
		}
		if werr != nil {
			return werr
		}
	}
	return nil
}
