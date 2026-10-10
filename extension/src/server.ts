// Talks to the display server (`srwr view-server`, docs/reference/protocol.md): a child process, newline-delimited
// JSON-RPC 2.0 on its standard input and output. This file does not import vscode, so tests can run it as it is.
import { ChildProcessWithoutNullStreams, spawn } from "node:child_process";
import { pick } from "./lang";
import { Frame, Hidden, ShownKind, toFrame } from "./timeline";

export const PROTOCOL_VERSION = 3;

export interface TapeInfo {
  tapeId: string;
  // The name of the tape file (`….tape.jsonl`, or `….tape.jsonl.gz` once closed). A server that does not say it: tapeId + ".tape.jsonl".
  file?: string;
  startedAt: string;
  updatedAt?: string;
  ops: number;
  files: string[];
  // The title and why the AI gave with the session tool (the header of the tape). Absent for a tape without them.
  title?: string;
  why?: string;
}

// What the display parts use to reach the server. Tests replace it with a fake (test/fakeserver.ts).
export interface ServerClient {
  listTapes(): Promise<TapeInfo[]>;
  // Opens a tape and returns the frames of the kinds asked for, with before/after (the final diff follows external), and
  // how many frames of each kind were left out.
  openTape(tapeId: string, kinds: ShownKind[]): Promise<{ frames: Frame[]; hidden: Hidden }>;
  closeTape(tapeId: string): Promise<void>;
  // Starts watching. Returns the frames so far; later ones arrive through onFrame. A frame with a tapeId that differs
  // from before means the live view moved to another tape.
  // onHidden gets the new count of the frames that were left out, when more of them arrive.
  liveStart(
    onFrame: (tapeId: string, frame: Frame) => void,
    kinds: ShownKind[],
    onHidden: (tapeId: string, hidden: Hidden) => void,
  ): Promise<{ tapeId: string | null; frames: Frame[]; hidden: Hidden }>;
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
  params?: { tapeId: string; frame: Record<string, unknown>; hidden?: Hidden };
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
  private onHidden: ((tapeId: string, hidden: Hidden) => void) | undefined;
  private disposed = false;

  constructor(private readonly cfg: ServerProcessOptions) {}

  async listTapes(): Promise<TapeInfo[]> {
    await this.initialize();
    const r = (await this.request("tapes/list", {})) as { tapes: TapeInfo[] };
    return r.tapes;
  }

  async openTape(tapeId: string, kinds: ShownKind[]): Promise<{ frames: Frame[]; hidden: Hidden }> {
    await this.initialize();
    const r = (await this.request("tape/open", { tapeId, withText: true, kinds })) as { frames: Record<string, unknown>[]; hidden?: Hidden };
    return { frames: r.frames.map(toFrame), hidden: r.hidden ?? {} };
  }

  async closeTape(tapeId: string): Promise<void> {
    await this.request("tape/close", { tapeId });
  }

  async liveStart(
    onFrame: (tapeId: string, frame: Frame) => void,
    kinds: ShownKind[],
    onHidden: (tapeId: string, hidden: Hidden) => void,
  ): Promise<{ tapeId: string | null; frames: Frame[]; hidden: Hidden }> {
    await this.initialize();
    this.onFrame = onFrame;
    this.onHidden = onHidden;
    const r = (await this.request("live/start", { withText: true, kinds })) as { tapeId: string | null; frames: Record<string, unknown>[]; hidden?: Hidden };
    return { tapeId: r.tapeId, frames: r.frames.map(toFrame), hidden: r.hidden ?? {} };
  }

  async liveStop(): Promise<void> {
    this.onFrame = undefined;
    this.onHidden = undefined;
    await this.request("live/stop", {});
  }

  dispose(): void {
    this.disposed = true;
    this.onFrame = undefined;
    this.onHidden = undefined;
    const child = this.child;
    this.child = undefined;
    this.failAll(new ServerError("server_exited", pick("the extension has shut down", "拡張は終了している")));
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
            pick(
              `srwr (${this.command}) and this extension do not match (the extension is protocolVersion ${PROTOCOL_VERSION}). Update one of them.`,
              `srwr（${this.command}）と、この拡張のバージョンが合っていません（拡張は protocolVersion ${PROTOCOL_VERSION}）。どちらかを更新してください。`,
            ),
          );
        }
        throw e;
      });
    return this.initialized;
  }

  private start(): Promise<void> {
    if (this.disposed) {
      return Promise.reject(new ServerError("server_exited", pick("the extension has shut down", "拡張は終了している")));
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
        reject(new ServerError("binary_not_found", pick(`Cannot start srwr (${this.command}): ${(e as Error).message}`, `srwr を起動できません（${this.command}）: ${(e as Error).message}`)));
        return;
      }
      let settled = false;
      child.once("spawn", () => {
        settled = true;
        this.child = child;
        resolve();
      });
      child.once("error", (e: NodeJS.ErrnoException) => {
        const err = new ServerError("binary_not_found", pick(`Cannot start srwr (${this.command}): ${e.code ?? e.message}`, `srwr を起動できません（${this.command}）: ${e.code ?? e.message}`));
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
        this.failAll(new ServerError("server_exited", pick(`The view server exited (${signal ?? `code ${code}`})${tail ? `\n${tail}` : ""}`, `表示サーバーが終了しました（${signal ?? `コード ${code}`}）${tail ? `\n${tail}` : ""}`)));
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
      return Promise.reject(new ServerError("server_exited", pick("the view server is not running", "表示サーバーが動いていない")));
    }
    const id = ++this.nextId;
    return new Promise<unknown>((resolve, reject) => {
      const timer = setTimeout(() => {
        this.pending.delete(id);
        reject(new ServerError("request_timeout", pick(`The view server does not respond (${method})`, `表示サーバーが応答しません（${method}）`)));
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
      this.cfg.log?.(`unreadable message: ${line.slice(0, 200)}`);
      return;
    }
    if (m.id === undefined) {
      if (m.method === "live/frame" && m.params) {
        this.onFrame?.(m.params.tapeId, toFrame(m.params.frame));
      } else if (m.method === "live/hidden" && m.params) {
        this.onHidden?.(m.params.tapeId, m.params.hidden ?? {});
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
