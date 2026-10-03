# The selection token

*[日本語](token_ja.md) | **English***

**Readers**: people who develop srwr. This document is not meant to be read alone. You come here by a link from [mcp.md](../reference/mcp.md) (the `sel_…` that `select` returns) and [tape.md](../reference/tape.md) (`from`, `selection`) when you want the details.

The **selection token** is a string that `select` returns and `replace` receives. It seals "which file, which lines, with which content were being looked at" in a single string, so the AI can edit by passing it on as it is.

```
sel_7K3M9QX2F4HD8R1WTB
└┬┘ └───────┬────────┘
prefix   Crockford Base32 (the binary below, encoded)
```

## Format

The binary inside (concatenated from the top):

| Field | Size | Content |
|---|---|---|
| version | 1 byte | Version of the format (`1`) |
| seq | variable-length integer | The sequence number, on the tape, of the event (`select` or `replace`) that issued the token |
| startLine | variable-length integer | First line |
| endLine | variable-length integer | Last line (`startLine - 1` for an empty range) |
| fileHash | 4 bytes | The first 4 bytes of the SHA-256 of the file's path relative to the workspace |
| textHash | 4 bytes | The first 4 bytes of the SHA-256 of the text in the range. The text is the lines of the range joined with `\n` (no trailing line break; the same form as `oldText` on the tape). An empty string for an empty range |
| mac | 3 bytes | The first 3 bytes of the HMAC-SHA256 (the key is `.srwr/key`) over the **tape ID** (the file name of the tape without `.tape.jsonl`, for example `20261001-1706-1795`) and everything above |

- The variable-length integer is LEB128 (the same scheme as the varint of Protocol Buffers)
- The total is roughly 14 to 20 bytes, which is **about 23 to 32 characters** in Base32
- The tape ID is used only to compute the mac and is not in the token. So a token issued on another tape (another session) has an HMAC that does not match and gives `invalid_selection`. The short ID (4 characters) alone could collide with another tape, so the whole tape ID, which includes the date and time, is used

## What is accepted when decoding

- Upper and lower case are not distinguished
- As the Crockford Base32 convention says, `O` is read as `0`, and `I` and `L` as `1`
- Separating hyphens are ignored

## Verification and correcting line numbers

When srwr receives a `replace`, it works in this order (the whole order is in [mcp.md](../reference/mcp.md)).

1. **Decode and verify the HMAC**: if it fails, `invalid_selection`
2. **Find the file**: look the target file up in the "path → fileHash" table built from the `snapshot`s of the tape
3. **Correct the line numbers**: follow, in order, the `replace` events made on the same file after the `seq` of the token. Let the range of the token be `[a, b]` and the (uncorrected) range of that edit be `[s, e]` (`e = s - 1` for an empty range)
   - `e < a`: the edit is above the range. Shift `a` and `b` by "the number of lines after the replacement − the number of lines before it"
   - `s > b`: the edit is below the range. Do nothing
   - Otherwise: the edit overlaps the range (this includes an insertion of an empty range that falls inside the range). It cannot be corrected, so `selection_stale`
4. **Check the content**: if the hash of the current text of the corrected range does not match `textHash`, `selection_mismatch`
5. **Execute**: replace, append to the tape, and return a new token

All edits to files are made by srwr itself, so srwr knows everything that happened after the token was issued. That is why the AI does not need to calculate line numbers.

A token issued before an `external` (a change outside srwr) cannot be followed by correcting line numbers. The content check gives `selection_mismatch`. srwr does not estimate the shift of lines from the content of the external change.

## Where it is valid

A token is valid only within the tape (session) that issued it. The `seq` in the token is a sequence number on the tape, and on another tape the same number points to another operation. When the session changes, the AI calls `select` again.

## Reasons for the choices

| Choice | Reason |
|---|---|
| One string | It is more certain for the AI to pass on one string than to copy a JSON object. It also looks cleaner on the tape |
| **Crockford Base32** | Only upper-case letters and digits, with confusing characters such as `0/O` and `1/I/L` removed. Case is not distinguished, so it resists copying mistakes by an LLM (the same scheme as ULID). base64 mixes cases, which increases the number of tokens and makes one-character slips easy, so it was not adopted |
| **Variable-length integer** | Line numbers and seq are often small, so a few bytes are enough and the token is short |
| **HMAC** | Even if the AI decodes the token and rewrites the line numbers on its own, it is detected. Both issuing and verifying are done inside srwr, so a heavy scheme such as JWT is not needed |
| The `sel_` prefix | A Stripe-style ID notation. It shows at a glance what kind of string it is, and taking another kind of ID by mistake is noticed at once |
| A human-readable form (`main.go:12-14@9f2c`) was not adopted | It is handy for reading the tape, but it tempts the AI to rewrite the line-number part and use it, which defeats the aim of "pass the return value of `select` as it is". For people, the decoded values are written alongside on the tape |

The length of the HMAC (3 bytes) is a trade-off between the rate of catching copying mistakes and the number of characters. Whether to review it is not decided ([limitations.md](limitations.md)).
