// The status bar: back, forward and the position are always there (a side that cannot be used is dimmed, not hidden).
// Live adds "● LIVE" while following, and "LIVE に戻る（新着 N）" while looking at an older frame. There is no auto play.
import * as vscode from "vscode";
import { pick } from "./lang";

// What the status bar shows. A replay session and the live view implement it.
export interface Nav {
  readonly onDidChange: vscode.Event<void>;
  readonly title: string;
  position(): { index: number; length: number }; // index is 0-based, -1 before any frame
  canBack(): boolean;
  canForward(): boolean;
  // Live only: not undefined means live. 0 is following the latest, N is looking at an older frame with N new ones.
  behind(): number | undefined;
  readonly closeCommand: string;
}

const DIM = "rgba(255, 255, 255, 0.4)";

export class Controls implements vscode.Disposable {
  private readonly back = this.item(`$(chevron-left) ${pick("Back", "戻る")}`, "srwr.stepBack", pick("Step back one frame", "1つ戻る（コマ送り）"), 106);
  private readonly forward = this.item(`${pick("Forward", "進む")} $(chevron-right)`, "srwr.stepForward", pick("Step forward one frame", "1つ進む（コマ送り）"), 105);
  private readonly pos = this.item("", undefined, "", 104);
  private readonly live = this.item("", "srwr.liveLatest", "", 103);
  private readonly close = this.item("$(close)", undefined, pick("Close", "閉じる"), 100);
  private readonly all = [this.back, this.forward, this.pos, this.live, this.close];

  private nav: Nav | undefined;
  private sub: vscode.Disposable | undefined;

  bind(nav: Nav | undefined): void {
    this.sub?.dispose();
    this.nav = nav;
    this.sub = nav?.onDidChange(() => this.update());
    this.update();
  }

  private update(): void {
    const n = this.nav;
    if (!n) {
      for (const it of this.all) {
        it.hide();
      }
      return;
    }
    const p = n.position();
    this.back.color = n.canBack() ? undefined : DIM;
    this.forward.color = n.canForward() ? undefined : DIM;
    this.pos.text = `${Math.max(p.index + 1, 0)}/${p.length}`;
    this.pos.tooltip = n.title;
    this.close.command = n.closeCommand;
    const behind = n.behind();
    if (behind === undefined) {
      this.live.hide();
    } else if (behind === 0) {
      this.live.text = "● LIVE";
      this.live.tooltip = pick("Following the latest frame", "最新のコマを追っている");
      this.live.backgroundColor = undefined;
      this.live.show();
    } else {
      this.live.text = pick(`Back to LIVE (${behind} new)`, `LIVE に戻る（新着 ${behind}）`);
      this.live.tooltip = pick("Jump to the latest frame and follow it again", "最新のコマへ移り、また追いかける");
      this.live.backgroundColor = new vscode.ThemeColor("statusBarItem.warningBackground");
      this.live.show();
    }
    for (const it of [this.back, this.forward, this.pos, this.close]) {
      it.show();
    }
  }

  private item(text: string, command: string | undefined, tooltip: string, priority: number): vscode.StatusBarItem {
    const it = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Left, priority);
    it.text = text;
    it.command = command;
    it.tooltip = tooltip;
    return it;
  }

  dispose(): void {
    this.sub?.dispose();
    for (const it of this.all) {
      it.dispose();
    }
  }
}
