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
//   - A file that is not what the tape says: an external event with the new content, then a snapshot.
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
		err := tx.Append(tape.Event{
			Type: tape.TypeExternal, Seq: tx.NextSeq(), File: rel, Author: &tape.Author{Kind: "external"},
			DetectedBy: detectedBy, ExpectedSha: tape.Sha(known.Text), ActualSha: tape.Sha(*current), Text: current,
		})
		if err != nil {
			return err
		}
		return appendSnapshot(tx, rel, *current)
	}
	return nil
}

func appendSnapshot(tx *session.Tx, rel, text string) error {
	return tx.Append(tape.Event{
		Type: tape.TypeSnapshot, Seq: tx.NextSeq(), File: rel,
		FileHash: tape.FileHash(rel), Text: &text, Sha: tape.Sha(text),
	})
}
