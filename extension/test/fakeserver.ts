import { ServerClient, TapeInfo } from "../src/server";
import { Frame, Hidden, ShownKind } from "../src/timeline";

// A server that answers from frames given to it. Live frames are pushed by the test with emit().
export class FakeServer implements ServerClient {
  calls: string[] = [];
  tapes: TapeInfo[] = [];
  frames = new Map<string, Frame[]>();
  liveTape: string | null = null;
  liveFrames: Frame[] = [];
  hiddenOf = new Map<string, Hidden>(); // what a tape says it left out
  openKinds: ShownKind[][] = []; // the kinds asked for in each open / liveStart, in order
  liveHidden: Hidden = {};
  failOpen: Error | undefined;
  failList: Error | undefined;
  private onFrame: ((tapeId: string, f: Frame) => void) | undefined;
  private onHidden: ((tapeId: string, h: Hidden) => void) | undefined;
  // What a test makes of the kinds asked for: given the frames of the tape and the kinds, the frames to send (the default keeps all).
  filter: ((frames: Frame[], kinds: ShownKind[]) => { frames: Frame[]; hidden: Hidden }) | undefined;
  // liveStart waits for this before it answers, to test notifications that come before the answer.
  liveGate: Promise<void> | undefined;
  beforeAnswer: (() => void) | undefined;

  async listTapes(): Promise<TapeInfo[]> {
    this.calls.push("list");
    if (this.failList) {
      throw this.failList;
    }
    return this.tapes;
  }

  async openTape(tapeId: string, kinds: ShownKind[]): Promise<{ frames: Frame[]; hidden: Hidden }> {
    this.calls.push(`open ${tapeId}`);
    this.openKinds.push(kinds);
    if (this.failOpen) {
      throw this.failOpen;
    }
    const frames = this.frames.get(tapeId) ?? [];
    return this.filter ? this.filter(frames, kinds) : { frames, hidden: this.hiddenOf.get(tapeId) ?? {} };
  }

  async closeTape(tapeId: string): Promise<void> {
    this.calls.push(`close ${tapeId}`);
  }

  async liveStart(
    onFrame: (tapeId: string, f: Frame) => void,
    kinds: ShownKind[],
    onHidden: (tapeId: string, h: Hidden) => void,
  ): Promise<{ tapeId: string | null; frames: Frame[]; hidden: Hidden }> {
    this.calls.push("liveStart");
    this.openKinds.push(kinds);
    this.onFrame = onFrame;
    this.onHidden = onHidden;
    this.beforeAnswer?.();
    await this.liveGate;
    const r = this.filter ? this.filter(this.liveFrames, kinds) : { frames: this.liveFrames, hidden: this.liveHidden };
    return { tapeId: this.liveTape, frames: r.frames, hidden: r.hidden };
  }

  async liveStop(): Promise<void> {
    this.calls.push("liveStop");
    this.onFrame = undefined;
    this.onHidden = undefined;
  }

  dispose(): void {
    this.calls.push("dispose");
  }

  emit(tapeId: string, f: Frame): void {
    this.onFrame?.(tapeId, f);
  }

  emitHidden(tapeId: string, h: Hidden): void {
    this.onHidden?.(tapeId, h);
  }

  get watching(): boolean {
    return this.onFrame !== undefined;
  }
}
