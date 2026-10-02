// Run with `node --require`: makes `require("vscode")` give the fake (test/fakevscode.ts).
import Module from "node:module";
import * as fake from "./fakevscode";

const m = Module as unknown as { _load: (request: string, ...rest: unknown[]) => unknown };
const load = m._load;
m._load = function (request: string, ...rest: unknown[]): unknown {
  return request === "vscode" ? fake : load.call(this, request, ...rest);
};
