// Run with `node --require`: makes `require("vscode")` give the fake (test/fakevscode.ts).
import Module from "node:module";

// The fixed tapes hold "+09:00" times and the screens list them in the time zone of the machine; the baselines were taken in this
// zone, so every run (a laptop, CI) uses it. A test that is about the zone sets process.env.TZ itself.
process.env.TZ = "Asia/Tokyo";
import * as fake from "./fakevscode";

const m = Module as unknown as { _load: (request: string, ...rest: unknown[]) => unknown };
const load = m._load;
m._load = function (request: string, ...rest: unknown[]): unknown {
  return request === "vscode" ? fake : load.call(this, request, ...rest);
};
