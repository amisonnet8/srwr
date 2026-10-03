// Times are kept in UTC (the tape and the view server write "…Z"); a person is shown the time in the time zone of the machine.
// This file does not import vscode.

// localStamp turns an RFC 3339 time into "2026-10-03 17:12:10" in the time zone of the machine. It returns undefined for text
// that is not a time.
export function localStamp(rfc3339: string): string | undefined {
  const d = new Date(rfc3339);
  if (Number.isNaN(d.getTime())) {
    return undefined;
  }
  const p = (n: number): string => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`;
}
