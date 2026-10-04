package timeline

// Kinds is the set of kinds of frames a client wants: select, replace, external and failure. The frames of the final diff
// (final) follow external, and those of sub follow replace.
type Kinds map[string]bool

// AllKinds are the names a client may ask for.
var AllKinds = []string{KindSelect, KindReplace, KindExternal, KindFailure}

// DefaultKinds is what a client gets when it does not say: everything but failure.
func DefaultKinds() Kinds {
	return Kinds{KindSelect: true, KindReplace: true, KindExternal: true}
}

// NewKinds makes a Kinds from names, and reports the first name that is not one of AllKinds.
func NewKinds(names []string) (Kinds, string) {
	k := Kinds{}
	for _, n := range names {
		known := false
		for _, a := range AllKinds {
			known = known || n == a
		}
		if !known {
			return nil, n
		}
		k[n] = true
	}
	return k, ""
}

// group is the kind a person turns on and off for a kind of frame: final goes with external, sub with replace.
func group(kind string) string {
	switch kind {
	case KindFinal:
		return KindExternal
	case KindSub:
		return KindReplace
	}
	return kind
}

// Shows says whether a frame of this kind is sent.
func (k Kinds) Shows(kind string) bool {
	return k[group(kind)]
}

// Hidden counts, for each kind that is left out, how many frames were. final counts as external. A kind with none is not in it.
type Hidden map[string]int

// Filter returns the frames the client wants, numbered from 0 again, and for each of them the index it has in frames (so that
// the text of a file at that frame can be found among all the frames, the hidden ones included), and how many were left out.
func Filter(frames []Frame, k Kinds) (shown []Frame, orig []int, hidden Hidden) {
	shown, orig, hidden = []Frame{}, []int{}, Hidden{}
	for i, f := range frames {
		if k.Shows(f.Kind) {
			f.Index = len(shown)
			shown = append(shown, f)
			orig = append(orig, i)
			continue
		}
		hidden[group(f.Kind)]++
	}
	return shown, orig, hidden
}
