package core

import (
	"github.com/amisonnet8/srwr/internal/session"
	"github.com/amisonnet8/srwr/internal/tape"
)

// Observe compares a file with what the tape knows of it, and records the difference. current is
// the content now, or nil if the file does not exist. detectedBy is what noticed it ("select",
// "replace", "hook").
//
//   - A file the tape has no content of: a snapshot, the first time it is touched.
//   - A file that is not what the tape says: an external event with the lines that changed (or, when
//     those cannot tell the change, the whole new content). No snapshot follows: the event is enough.
//   - A file that is gone: an external event that says so. When it comes back, a snapshot.
//
// A file the tape has no content of is never external: there is nothing to compare with.
func Observe(tx *session.Tx, rel, detectedBy string, current *string) error {
	known := tx.State().Files[rel]
	switch {
	case current == nil:
		if known == nil || known.Deleted {
			return nil
		}
		return tx.Append(tape.Event{
			Type: tape.TypeExternal, Seq: tx.NextSeq(), File: rel, Author: &tape.Author{Kind: "external"},
			DetectedBy: detectedBy, ExpectedSha: tape.Sha(known.Text), Deleted: true,
		})
	case known == nil || known.Deleted:
		return appendSnapshot(tx, rel, *current)
	case known.Text != *current:
		e := tape.Event{
			Type: tape.TypeExternal, Seq: tx.NextSeq(), File: rel, Author: &tape.Author{Kind: "external"},
			DetectedBy: detectedBy, ExpectedSha: tape.Sha(known.Text), ActualSha: tape.Sha(*current),
		}
		// Lines are not enough when only the end of the file changed (a final line break), or when
		// too much changed to compare: then the whole text goes on the tape.
		if hunks, ok := tape.Diff(known.Text, *current); ok && tape.ApplyHunks(known.Text, hunks) == *current {
			e.Hunks = hunks
		} else {
			e.Text = current
		}
		return tx.Append(e)
	}
	return nil
}

func appendSnapshot(tx *session.Tx, rel, text string) error {
	return tx.Append(tape.Event{
		Type: tape.TypeSnapshot, Seq: tx.NextSeq(), File: rel,
		FileHash: tape.FileHash(rel), Text: &text, Sha: tape.Sha(text),
	})
}

// observeCreated records a new file: one the tape had no content of, which appeared. It is an external event with Created set,
// told as lines added to an empty file (or, for an empty file, as the empty text).
func observeCreated(tx *session.Tx, rel, detectedBy, text string) error {
	e := tape.Event{
		Type: tape.TypeExternal, Seq: tx.NextSeq(), File: rel, Author: &tape.Author{Kind: "external"},
		DetectedBy: detectedBy, ActualSha: tape.Sha(text), Created: true,
	}
	if hunks, ok := tape.Diff("", text); ok && len(hunks) > 0 && tape.ApplyHunks("", hunks) == text {
		e.Hunks = hunks
	} else {
		e.Text = &text
	}
	return tx.Append(e)
}
