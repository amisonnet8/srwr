import { ServerClient, TapeInfo } from "../src/server";
import { Frame } from "../src/timeline";

// A server that answers from frames given to it. Live frames are pushed by the test with emit().
export class FakeServer implements ServerClient {
  calls: string[] = [];
  tapes: TapeInfo[] = [];
  frames = new Map<string, Frame[]>();
  liveTape: string | null = null;
  liveFrames: Frame[] = [];
  failOpen: Error | undefined;
  failList: Error | undefined;
  private onFrame: ((tapeId: string, f: Frame) => void) | undefined;
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

  async openTape(tapeId: string): Promise<Frame[]> {
    this.calls.push(`open ${tapeId}`);
    if (this.failOpen) {
      throw this.failOpen;
    }
    return this.frames.get(tapeId) ?? [];
  }

  async closeTape(tapeId: string): Promise<void> {
    this.calls.push(`close ${tapeId}`);
  }

  async liveStart(onFrame: (tapeId: string, f: Frame) => void): Promise<{ tapeId: string | null; frames: Frame[] }> {
    this.calls.push("liveStart");
    this.onFrame = onFrame;
    this.beforeAnswer?.();
    await this.liveGate;
    return { tapeId: this.liveTape, frames: this.liveFrames };
  }

  async liveStop(): Promise<void> {
    this.calls.push("liveStop");
    this.onFrame = undefined;
  }

  dispose(): void {
    this.calls.push("dispose");
  }

  emit(tapeId: string, f: Frame): void {
    this.onFrame?.(tapeId, f);
  }

  get watching(): boolean {
    return this.onFrame !== undefined;
  }
}
