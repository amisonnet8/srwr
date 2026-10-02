// Talks to the display server (`srwr view-server`, docs/reference/protocol.md): a child process, newline-delimited
// JSON-RPC 2.0 on its standard input and output. This file does not import vscode, so tests can run it as it is.
import { ChildProcessWithoutNullStreams, spawn } from "node:child_process";
import { Frame, toFrame } from "./timeline";

export const PROTOCOL_VERSION = 1;

export interface TapeInfo {
  tapeId: string;
  startedAt: string;
  updatedAt?: string;
  ops: number;
  files: string[];
}

// What the display parts use to reach the server. Tests replace it with a fake (test/fakeserver.ts).
export interface ServerClient {
  listTapes(): Promise<TapeInfo[]>;
  // Opens a tape and returns all its frames with before/after, including the final diff.
  openTape(tapeId: string): Promise<Frame[]>;
  closeTape(tapeId: string): Promise<void>;
  // Starts watching. Returns the frames so far; later ones arrive through onFrame. A frame with a tapeId that differs
  // from before means the live view moved to another tape.
  liveStart(onFrame: (tapeId: string, frame: Frame) => void): Promise<{ tapeId: string | null; frames: Frame[] }>;
  liveStop(): Promise<void>;
  dispose(): void;
}

// The server's own error codes (tape_not_found ...) are passed through as `code`.
export type ServerErrorCode = "binary_not_found" | "server_exited" | "protocol_mismatch" | "request_timeout" | string;

export class ServerError extends Error {
  constructor(
    readonly code: ServerErrorCode,
    message: string,
  ) {
    super(message);
    this.name = "ServerError";
  }
}

// The setting srwr.path wins unless it is the default "srwr"; then the environment variable SRWR_PATH (development);
// then "srwr" from PATH.
export function resolveCommand(settingPath: string, env: NodeJS.ProcessEnv): string {
  const set = settingPath.trim();
  if (set !== "" && set !== "srwr") {
    return set;
  }
  const fromEnv = (env.SRWR_PATH ?? "").trim();
  return fromEnv !== "" ? fromEnv : "srwr";
}

export interface ServerProcessOptions {
  root: string;
  command: () => string;
  args?: string[]; // for tests
  timeoutMs?: number;
  log?: (line: string) => void;
}

interface Pending {
  method: string;
  resolve: (v: unknown) => void;
  reject: (e: Error) => void;
  timer: NodeJS.Timeout;
}

interface Message {
  id?: number;
  method?: string;
  params?: { tapeId: string; frame: Record<string, unknown> };
  result?: unknown;
  error?: { message: string; data?: { code?: string } };
}

export class ServerProcess implements ServerClient {
  private child: ChildProcessWithoutNullStreams | undefined;
  private starting: Promise<void> | undefined;
  private initialized: Promise<void> | undefined;
  private command = "";
  private buffer = "";
  private stderrTail: string[] = [];
  private nextId = 0;
  private readonly pending = new Map<number, Pending>();
  private onFrame: ((tapeId: string, frame: Frame) => void) | undefined;
  private disposed = false;

  constructor(private readonly cfg: ServerProcessOptions) {}

  async listTapes(): Promise<TapeInfo[]> {
    await this.initialize();
    const r = (await this.request("tapes/list", {})) as { tapes: TapeInfo[] };
    return r.tapes;
  }

  async openTape(tapeId: string): Promise<Frame[]> {
    await this.initialize();
    const r = (await this.request("tape/open", { tapeId, withText: true })) as { frames: Record<string, unknown>[] };
    return r.frames.map(toFrame);
  }

  async closeTape(tapeId: string): Promise<void> {
    await this.request("tape/close", { tapeId });
  }

  async liveStart(onFrame: (tapeId: string, frame: Frame) => void): Promise<{ tapeId: string | null; frames: Frame[] }> {
    await this.initialize();
    this.onFrame = onFrame;
    const r = (await this.request("live/start", { withText: true })) as { tapeId: string | null; frames: Record<string, unknown>[] };
    return { tapeId: r.tapeId, frames: r.frames.map(toFrame) };
  }

  async liveStop(): Promise<void> {
    this.onFrame = undefined;
    await this.request("live/stop", {});
  }

  dispose(): void {
    this.disposed = true;
    this.onFrame = undefined;
    const child = this.child;
    this.child = undefined;
    this.failAll(new ServerError("server_exited", "拡張は終了している"));
    if (child) {
      child.stdin.write(JSON.stringify({ jsonrpc: "2.0", id: ++this.nextId, method: "shutdown", params: {} }) + "\n");
      child.stdin.end();
      setTimeout(() => child.kill(), 1000).unref();
    }
  }

  // Starts the process and sends initialize once per process.
  private initialize(): Promise<void> {
    this.initialized ??= this.start()
      .then(() => this.request("initialize", { client: "vscode", protocolVersion: PROTOCOL_VERSION, options: { diffFrames: true } }))
      .then(() => undefined)
      .catch((e: unknown) => {
        this.initialized = undefined;
        if (e instanceof ServerError && e.code === "protocol_mismatch") {
          throw new ServerError(
            "protocol_mismatch",
            `srwr（${this.command}）と、この拡張のバージョンが合っていません（拡張は protocolVersion ${PROTOCOL_VERSION}）。どちらかを更新してください。`,
          );
        }
        throw e;
      });
    return this.initialized;
  }

  private start(): Promise<void> {
    if (this.disposed) {
      return Promise.reject(new ServerError("server_exited", "拡張は終了している"));
    }
    if (this.child) {
      return Promise.resolve();
    }
    this.starting ??= this.spawnChild().finally(() => {
      this.starting = undefined;
    });
    return this.starting;
  }

  private spawnChild(): Promise<void> {
    return new Promise<void>((resolve, reject) => {
      this.command = this.cfg.command();
      const args = this.cfg.args ?? ["view-server", "--root", this.cfg.root];
      let child: ChildProcessWithoutNullStreams;
      try {
        child = spawn(this.command, args, { stdio: ["pipe", "pipe", "pipe"], windowsHide: true });
      } catch (e) {
        reject(new ServerError("binary_not_found", `srwr を起動できません（${this.command}）: ${(e as Error).message}`));
        return;
      }
      let settled = false;
      child.once("spawn", () => {
        settled = true;
        this.child = child;
        resolve();
      });
      child.once("error", (e: NodeJS.ErrnoException) => {
        const err = new ServerError("binary_not_found", `srwr を起動できません（${this.command}）: ${e.code ?? e.message}`);
        if (!settled) {
          settled = true;
          reject(err);
        }
        this.child = undefined;
        this.initialized = undefined;
        this.failAll(err);
      });
      child.once("exit", (code, signal) => {
        if (this.child === child) {
          this.child = undefined;
          this.initialized = undefined;
        }
        this.buffer = "";
        const tail = this.stderrTail.join("\n");
        this.failAll(new ServerError("server_exited", `表示サーバーが終了しました（${signal ?? `コード ${code}`}）${tail ? `\n${tail}` : ""}`));
      });
      child.stdout.setEncoding("utf8");
      child.stdout.on("data", (chunk: string) => this.onData(chunk));
      child.stderr.setEncoding("utf8");
      child.stderr.on("data", (chunk: string) => {
        for (const line of chunk.split("\n").filter(Boolean)) {
          this.stderrTail = [...this.stderrTail, line].slice(-5);
          this.cfg.log?.(line);
        }
      });
      // Writing to a server that already ended gives EPIPE; the exit handler reports that.
      child.stdin.on("error", () => undefined);
    });
  }

  private request(method: string, params: unknown): Promise<unknown> {
    const child = this.child;
    if (!child) {
      return Promise.reject(new ServerError("server_exited", "表示サーバーが動いていない"));
    }
    const id = ++this.nextId;
    return new Promise<unknown>((resolve, reject) => {
      const timer = setTimeout(() => {
        this.pending.delete(id);
        reject(new ServerError("request_timeout", `表示サーバーが応答しません（${method}）`));
      }, this.cfg.timeoutMs ?? 15000);
      this.pending.set(id, { method, resolve, reject, timer });
      child.stdin.write(JSON.stringify({ jsonrpc: "2.0", id, method, params }) + "\n");
    });
  }

  private onData(chunk: string): void {
    this.buffer += chunk;
    for (;;) {
      const i = this.buffer.indexOf("\n");
      if (i < 0) {
        return;
      }
      const line = this.buffer.slice(0, i);
      this.buffer = this.buffer.slice(i + 1);
      if (line.trim() !== "") {
        this.onMessage(line);
      }
    }
  }

  private onMessage(line: string): void {
    let m: Message;
    try {
      m = JSON.parse(line) as Message;
    } catch {
      this.cfg.log?.(`読めないメッセージ: ${line.slice(0, 200)}`);
      return;
    }
    if (m.id === undefined) {
      if (m.method === "live/frame" && m.params) {
        this.onFrame?.(m.params.tapeId, toFrame(m.params.frame));
      }
      return;
    }
    const p = this.pending.get(m.id);
    if (!p) {
      return;
    }
    this.pending.delete(m.id);
    clearTimeout(p.timer);
    if (m.error) {
      p.reject(new ServerError(m.error.data?.code ?? "server_error", m.error.message));
    } else {
      p.resolve(m.result);
    }
  }

  private failAll(err: Error): void {
    for (const [id, p] of this.pending) {
      this.pending.delete(id);
      clearTimeout(p.timer);
      p.reject(err);
    }
  }
}
