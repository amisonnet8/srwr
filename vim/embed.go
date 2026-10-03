// Package vim holds srwr-view.vim, the Vim client. srwr embeds it, and `srwr view` writes it out to the
// user's cache directory and starts Vim with it (docs/reference/vim.md).
package vim

import "embed"

// Files is plugin/ and autoload/. Anything else under vim/ (the tests) is not part of what a user runs.
//
//go:embed plugin autoload
var Files embed.FS
