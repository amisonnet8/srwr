// The language of what the extension shows: English, or Japanese when VSCode itself is in Japanese
// (vscode.env.language). This file does not import vscode, so server.ts can use it.
let japanese = false;

export function setJapanese(on: boolean): void {
  japanese = on;
}

export function isJapanese(): boolean {
  return japanese;
}

// pick returns the Japanese text when the screen is Japanese, the English text otherwise.
export function pick(en: string, ja: string): string {
  return japanese ? ja : en;
}
