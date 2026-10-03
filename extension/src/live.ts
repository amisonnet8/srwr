// Live: the same screen as replay, but the frames keep arriving. The view follows the latest frame unless the user went back.
import * as vscode from "vscode";
import { pick } from "./lang";
import { Nav } from "./controls";
import { Presenter } from "./present";
import { ReplayProvider, ReplaySession } from "./replay";
import { ServerClient } from "./server";
import { OpsSource } from "./sidebar";
import { Frame, Timeline } from "./timeline";

export class LiveView implements OpsSource, Nav, vscode.Disposable {
  readonly title = pick("Live view", "ライブ視聴");
  readonly closeCommand = "srwr.liveStop";

  private readonly session: ReplaySession;
  private tapeId: string | null = null; // the tape being followed; a frame of another tape means the view moved
  private started = false;
  // Frames that arrived before the response of live/start was handled are kept and played after it.
  private early: Array<[string, Frame]> | undefined = [];
  private queue: Promise<void> = Promise.resolve();
  // The frame we decided to show. The screen catches up later, so "are we following" is decided from this, not from the screen.
  private wanted = -1;
  private disposed = false;

  constructor(
    presenter: Presenter,
    private readonly server: ServerClient,
    provider: ReplayProvider,
  ) {
    this.session = new ReplaySession("live", [], provider, presenter);
  }

  get timeline(): Timeline {
    return this.session.timeline;
  }

  get index(): number {
    return this.session.index;
  }

  get onDidChange(): vscode.Event<void> {
    return this.session.onDidChange;
  }

  current(): number {
    return this.session.index;
  }

  position(): { index: number; length: number } {
    return this.session.position();
  }

  canBack(): boolean {
    return this.session.canBack();
  }

  canForward(): boolean {
    return this.session.canForward();
  }

  // How many frames arrived after the one on the screen. 0 means following the latest.
  behind(): number {
    return Math.max(this.timeline.length - 1 - this.session.index, 0);
  }

  // Asks the server to watch. The frames up to now are only listed, not shown. Frames appended later, and frames of a
  // tape that shows up later (from its start), arrive as notifications and are shown.
  async start(): Promise<void> {
    const r = await this.server.liveStart((id, f) => this.onFrame(id, f));
    if (this.disposed) {
      // Closed while waiting. The server has started watching, so it is asked to stop.
      this.server.liveStop().catch(() => undefined);
      return;
    }
    this.started = true;
    this.tapeId = r.tapeId;
    for (const f of r.frames) {
      this.timeline.append(f);
    }
    this.wanted = r.frames.length - 1;
    this.session.setIndex(this.wanted);
    const early = this.early ?? [];
    this.early = undefined;
    for (const [id, f] of early) {
      this.onFrame(id, f);
    }
  }

  dispose(): void {
    this.disposed = true;
    if (this.started) {
      this.server.liveStop().catch(() => undefined); // the server may already be gone
    }
    this.session.dispose();
  }

  stepForward(): Promise<void> {
    return this.show(Math.min(this.wanted + 1, this.timeline.length - 1));
  }

  stepBack(): Promise<void> {
    return this.show(Math.max(this.wanted - 1, 0));
  }

  // Resolves when the screen has caught up with every frame we decided to show.
  settled(): Promise<void> {
    return this.queue;
  }

  // Back to the latest frame, and follow again.
  latest(): Promise<void> {
    return this.show(this.timeline.length - 1);
  }

  jump(i: number): Promise<void> {
    return i >= 0 && i < this.timeline.length ? this.show(i) : this.queue;
  }

  // Decides to show frame i, then updates the screen one step at a time.
  private show(i: number): Promise<void> {
    if (i < 0 || this.timeline.length === 0) {
      return this.queue;
    }
    this.wanted = i;
    return this.enqueue(() => this.session.goto(i));
  }

  private enqueue(f: () => Promise<void>): Promise<void> {
    this.queue = this.queue.then(f).catch((e) => console.error("srwr-view:", e));
    return this.queue;
  }

  private onFrame(tapeId: string, f: Frame): void {
    if (this.disposed) {
      return;
    }
    if (this.early) {
      this.early.push([tapeId, f]);
      return;
    }
    if (tapeId !== this.tapeId) {
      this.tapeId = tapeId;
      this.session.reset();
      this.wanted = -1;
    }
    const following = this.wanted === this.timeline.length - 1;
    this.session.append(f);
    if (following) {
      void this.show(this.timeline.length - 1);
    }
  }
}
