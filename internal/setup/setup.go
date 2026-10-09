// Package setup is srwr init: it prepares a workspace so that Claude Code uses srwr (docs/reference/cli.md).
// It adds to .mcp.json, .claude/settings.json and .gitignore without breaking what is already in them, and keeps a copy
// of every file it changes in .srwr/init-backup/.
package setup

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/amisonnet8/srwr/internal/lang"
	"github.com/amisonnet8/srwr/internal/session"
)

// Options of Init.
type Options struct {
	Root     string
	Lenient  bool                         // do not forbid Edit and Write
	Now      func() time.Time             // for the name of the backup; default time.Now
	LookPath func(string) (string, error) // for the check that srwr is on PATH; default exec.LookPath
}

// Kind says what happened to a file.
type Kind int

// What init did to a file.
const (
	Unchanged Kind = iota
	Created
	Appended
)

// Change is what init did to one file (or the key).
type Change struct {
	Path   string // slash-separated, from the workspace root
	Kind   Kind
	Detail string // what was added, in a short sentence for the person
}

// Result of Init.
type Result struct {
	Root       string
	Changes    []Change
	Backup     string // slash-separated path from the workspace root; "" if nothing was copied
	Lenient    bool
	SrwrOnPath bool
	// RegistrationChanged is true when srwr was registered (the key, .mcp.json or the hook): Claude Code has to be opened again.
	RegistrationChanged bool
}

// Changed reports whether anything was written.
func (r *Result) Changed() bool {
	for _, c := range r.Changes {
		if c.Kind != Unchanged {
			return true
		}
	}
	return false
}

// UserError is a mistake in the files of the workspace that init cannot go on with. Nothing has been changed.
type UserError struct{ Msg string }

func (e *UserError) Error() string { return e.Msg }

const (
	hookCommand  = "srwr hook"
	hookMatcher  = "Read|Bash|Grep|Edit"
	mcpName      = "srwr"
	backupDir    = ".srwr/init-backup"
	settingsPath = ".claude/settings.json"
	mcpPath      = ".mcp.json"
)

var (
	forbidden = []string{"Edit", "Write", "MultiEdit", "NotebookEdit"}
	allowed   = []string{"mcp__srwr__look", "mcp__srwr__edit", "mcp__srwr__new"}
	// retired are the permissions of tools that no longer exist (select and sub of version 0.1.4 and before, replace of 0.1.12 and
	// before).
	retired     = []string{"mcp__srwr__select", "mcp__srwr__sub", "mcp__srwr__replace"}
	ignoreLines = []string{".srwr/key", ".srwr/lock", ".srwr/active", ".srwr/init-backup/"}
)

// plan is one file to write, worked out before anything is written.
type plan struct {
	rel       string
	old       []byte // nil: the file did not exist
	text      []byte
	change    Change
	registers bool // the change registers srwr (the hook was added)
}

// Init prepares the workspace. If a file cannot be read (broken JSON), nothing is written and a *UserError is returned.
func Init(opts Options) (*Result, error) {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.LookPath == nil {
		opts.LookPath = exec.LookPath
	}
	root, err := filepath.Abs(opts.Root)
	if err != nil {
		return nil, err
	}
	res := &Result{Root: root, Lenient: opts.Lenient}

	var plans []plan
	mcp, err := planMCP(root)
	if err != nil {
		return nil, err
	}
	settings, err := planSettings(root, opts.Lenient)
	if err != nil {
		return nil, err
	}
	plans = append(plans, mcp, settings)
	gi, hasGit, err := planGitignore(root)
	if err != nil {
		return nil, err
	}

	// The key first in the list, as the person sees it.
	keyChange, err := ensureKey(root)
	if err != nil {
		return nil, err
	}
	res.Changes = append(res.Changes, keyChange)
	for _, p := range plans {
		res.Changes = append(res.Changes, p.change)
	}
	if hasGit {
		plans = append(plans, gi)
		res.Changes = append(res.Changes, gi.change)
	}

	var changed []plan
	for _, p := range plans {
		if p.change.Kind != Unchanged {
			changed = append(changed, p)
		}
	}
	if err := backup(root, changed, opts.Now(), res); err != nil {
		return nil, err
	}
	for _, p := range changed {
		if err := writeAtomic(filepath.Join(root, filepath.FromSlash(p.rel)), p.text); err != nil {
			return nil, err
		}
	}
	res.RegistrationChanged = keyChange.Kind != Unchanged || mcp.change.Kind != Unchanged || settings.registers
	_, lookErr := opts.LookPath("srwr")
	res.SrwrOnPath = lookErr == nil
	return res, nil
}

// ensureKey makes .srwr/ and the key in it. The session package makes the key the same way for srwr mcp.
func ensureKey(root string) (Change, error) {
	c := Change{Path: ".srwr/"}
	_, statErr := os.Stat(filepath.Join(root, ".srwr", "key"))
	existed := statErr == nil
	ws, err := session.Open(root, session.Options{})
	if err != nil {
		return c, err
	}
	if err := ws.Prepare(); err != nil {
		return c, err
	}
	if !existed {
		c.Kind = Created
		c.Detail = "key"
	}
	return c, nil
}

func readOptional(root, rel string) ([]byte, error) {
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel))) //nolint:gosec // a file of the workspace
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return b, err
}

// readObject reads a JSON file that must hold an object. A file that does not exist is an empty one (existed is false).
func readObject(root, rel string) (o *object, old []byte, existed bool, err error) {
	old, err = readOptional(root, rel)
	if err != nil {
		return nil, nil, false, err
	}
	if old == nil {
		return newObject(), nil, false, nil
	}
	v, perr := parseJSON(old)
	if perr != nil {
		msg := lang.Sprintf("%s is not valid JSON", "%s を JSON として読めません", rel)
		if n := lineOf(old, perr); n > 0 {
			msg += lang.Sprintf(" (near line %d)", "（%d 行目付近）", n)
		}
		return nil, nil, false, &UserError{Msg: msg}
	}
	obj, ok := v.(*object)
	if !ok {
		return nil, nil, false, &UserError{Msg: lang.Sprintf("the top level of %s is not a JSON object", "%s の一番外側が JSON のオブジェクトではありません", rel)}
	}
	return obj, old, true, nil
}

// child returns o[key] as an object, making it if it is not there. A value of another type is a mistake of the person.
func child(o *object, key, where string) (*object, error) {
	v, ok := o.get(key)
	if !ok {
		c := newObject()
		o.set(key, c)
		return c, nil
	}
	c, ok := v.(*object)
	if !ok {
		return nil, &UserError{Msg: lang.Sprintf("%s: %s is not an object", "%s の %s がオブジェクトではありません", where, key)}
	}
	return c, nil
}

// strings reads o[key] as a list of strings, making it if it is not there.
func stringList(o *object, key, where string) ([]any, error) {
	v, ok := o.get(key)
	if !ok {
		return []any{}, nil
	}
	l, ok := v.([]any)
	if !ok {
		return nil, &UserError{Msg: lang.Sprintf("%s: %s is not an array", "%s の %s が配列ではありません", where, key)}
	}
	return l, nil
}

func hasString(l []any, s string) bool {
	return slices.ContainsFunc(l, func(v any) bool { x, ok := v.(string); return ok && x == s })
}

func planMCP(root string) (plan, error) {
	o, old, existed, err := readObject(root, mcpPath)
	if err != nil {
		return plan{}, err
	}
	servers, err := child(o, "mcpServers", mcpPath)
	if err != nil {
		return plan{}, err
	}
	p := plan{rel: mcpPath, old: old, change: Change{Path: mcpPath}}
	if _, ok := servers.get(mcpName); ok {
		return p, nil
	}
	others := len(servers.keys)
	entry := newObject()
	entry.set("command", "srwr")
	entry.set("args", []any{"mcp"})
	servers.set(mcpName, entry)
	p.text, err = marshalJSON(o)
	if err != nil {
		return plan{}, err
	}
	p.change.Kind = Appended
	if !existed {
		p.change.Kind = Created
	}
	p.change.Detail = lang.Pick("registered srwr mcp", "srwr mcp を登録しました")
	if others > 0 {
		if others == 1 {
			p.change.Detail += lang.Sprintf(" (%d other server left as it was)", "（ほかのサーバー %d 件はそのまま）", others)
		} else {
			p.change.Detail += lang.Sprintf(" (%d other servers left as they were)", "（ほかのサーバー %d 件はそのまま）", others)
		}
	}
	return p, nil
}

func planSettings(root string, lenient bool) (plan, error) {
	const where = settingsPath
	o, old, existed, err := readObject(root, settingsPath)
	if err != nil {
		return plan{}, err
	}
	p := plan{rel: settingsPath, old: old, change: Change{Path: settingsPath}}
	var hookAdded, denyAdded, denyRemoved, allowAdded bool

	hooks, err := child(o, "hooks", where)
	if err != nil {
		return plan{}, err
	}
	post, err := stringListAny(hooks, "PostToolUse", where+lang.Pick(", hooks", " の hooks"))
	if err != nil {
		return plan{}, err
	}
	if !hasHookCommand(post) {
		entry := newObject()
		entry.set("matcher", hookMatcher)
		h := newObject()
		h.set("type", "command")
		h.set("command", hookCommand)
		entry.set("hooks", []any{h})
		hooks.set("PostToolUse", append(post, entry))
		hookAdded = true
	}

	enabled, err := stringList(o, "enabledMcpjsonServers", where)
	if err != nil {
		return plan{}, err
	}
	if !hasString(enabled, mcpName) {
		o.set("enabledMcpjsonServers", append(enabled, mcpName))
		allowAdded = true
	}

	perms, err := child(o, "permissions", where)
	if err != nil {
		return plan{}, err
	}
	allow, err := stringList(perms, "allow", where+lang.Pick(", permissions", " の permissions"))
	if err != nil {
		return plan{}, err
	}
	for _, a := range allowed {
		if !hasString(allow, a) {
			allow = append(allow, a)
			allowAdded = true
		}
	}
	allowKept := make([]any, 0, len(allow))
	for _, a := range allow {
		if str, ok := a.(string); ok && slices.Contains(retired, str) {
			allowAdded = true // the file changes
			continue
		}
		allowKept = append(allowKept, a)
	}
	perms.set("allow", allowKept)

	deny, err := stringList(perms, "deny", where+lang.Pick(", permissions", " の permissions"))
	if err != nil {
		return plan{}, err
	}
	if lenient {
		kept := make([]any, 0, len(deny))
		for _, d := range deny {
			if s, ok := d.(string); ok && slices.Contains(forbidden, s) {
				denyRemoved = true
				continue
			}
			kept = append(kept, d)
		}
		deny = kept
	} else {
		for _, f := range forbidden {
			if !hasString(deny, f) {
				deny = append(deny, f)
				denyAdded = true
			}
		}
	}
	if len(deny) == 0 {
		perms.remove("deny")
	} else {
		perms.set("deny", deny)
	}

	if !hookAdded && !denyAdded && !denyRemoved && !allowAdded {
		return p, nil
	}
	p.text, err = marshalJSON(o)
	if err != nil {
		return plan{}, err
	}
	p.change.Kind = Appended
	if !existed {
		p.change.Kind = Created
	}
	p.registers = hookAdded
	p.change.Detail = settingsDetail(!existed, lenient, hookAdded, denyAdded, denyRemoved)
	return p, nil
}

func settingsDetail(created, lenient, hookAdded, denyAdded, denyRemoved bool) string {
	switch {
	case created && lenient:
		return lang.Pick("registered the hook (lenient mode)", "hook を登録しました（緩いモード）")
	case created:
		return lang.Pick("registered the hook; forbade Edit, Write, etc. (strict mode)", "hook を登録し、Edit・Write などを禁止しました（厳格モード）")
	case !hookAdded && denyRemoved:
		return lang.Pick("lifted the ban on Edit, Write, etc. (lenient mode)", "Edit・Write などの禁止を外しました（緩いモード）")
	case !hookAdded && denyAdded:
		return lang.Pick("forbade Edit, Write, etc. (strict mode)", "Edit・Write などを禁止しました（厳格モード）")
	case !hookAdded:
		return lang.Pick("added the permissions (existing settings left as they were)", "許可を足しました（既存の設定はそのまま）")
	case lenient:
		return lang.Pick("added the hook (existing settings left as they were)", "hook を足しました（既存の設定はそのまま）")
	default:
		return lang.Pick("added the hook and the prohibitions (existing settings left as they were)", "hook と禁止を足しました（既存の設定はそのまま）")
	}
}

// stringListAny reads o[key] as a list of any.
func stringListAny(o *object, key, where string) ([]any, error) { return stringList(o, key, where) }

// hasHookCommand reports whether a PostToolUse list already calls srwr hook.
func hasHookCommand(post []any) bool {
	for _, e := range post {
		eo, ok := e.(*object)
		if !ok {
			continue
		}
		hv, _ := eo.get("hooks")
		hl, _ := hv.([]any)
		for _, h := range hl {
			ho, ok := h.(*object)
			if !ok {
				continue
			}
			if c, _ := ho.get("command"); c == hookCommand {
				return true
			}
		}
	}
	return false
}

// planGitignore adds the lines that keep the key and the like out of git. hasGit is false outside a git work tree.
func planGitignore(root string) (p plan, hasGit bool, err error) {
	if !inGit(root) {
		return plan{}, false, nil
	}
	old, err := readOptional(root, ".gitignore")
	if err != nil {
		return plan{}, true, err
	}
	p = plan{rel: ".gitignore", old: old, change: Change{Path: ".gitignore"}}
	have := map[string]bool{}
	for _, l := range strings.Split(string(old), "\n") {
		have[strings.TrimSpace(l)] = true
	}
	var add []string
	for _, l := range ignoreLines {
		if !have[l] {
			add = append(add, l)
		}
	}
	if len(add) == 0 {
		return p, true, nil
	}
	text := string(old)
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	text += strings.Join(add, "\n") + "\n"
	p.text = []byte(text)
	p.change.Kind = Appended
	if old == nil {
		p.change.Kind = Created
	}
	p.change.Detail = strings.Join(add, " ")
	return p, true, nil
}

// inGit reports whether dir is in a git work tree: it or a directory above it has .git.
func inGit(dir string) bool {
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return false
		}
		dir = parent
	}
}

// backup copies the files that are about to change (and exist) to .srwr/init-backup/<time>/.
func backup(root string, changed []plan, now time.Time, res *Result) error {
	var dir string
	for _, p := range changed {
		if p.old == nil {
			continue
		}
		if dir == "" {
			dir = backupDir + "/" + now.Format("20060102-150405")
		}
		dst := filepath.Join(root, filepath.FromSlash(dir), filepath.FromSlash(p.rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
			return err
		}
		if err := os.WriteFile(dst, p.old, 0o600); err != nil {
			return err
		}
	}
	res.Backup = dir
	return nil
}

// writeAtomic writes next to the file and renames, keeping the permissions of a file that is there.
func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	mode := fs.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".srwr-init-*.tmp")
	if err != nil {
		return err
	}
	fail := func(err error) error {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		return fail(err)
	}
	if err := tmp.Chmod(mode); err != nil {
		return fail(err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return nil
}
