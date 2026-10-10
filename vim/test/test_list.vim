vim9script
# The list of tapes (srwr://tapes).
import './helpers.vim' as t
t.Setup()
import autoload 'srwr/list.vim'
import autoload 'srwr/replay.vim'
import autoload 'srwr/server.vim'
import autoload 'srwr/ui.vim'

set columns=140 lines=50
t.Workspace()

ui.Open('')
t.WaitFor((): bool => list.Active(), 'the list')
const b = list.Buf()
t.Equal('srwr://tapes', bufname(b), 'name')
const rows = getbufline(b, 1, '$')
t.Equal('開始時刻          更新時刻            操作  ファイル  テープ', rows[0], 'heading')
t.Equal(4, len(rows), 'heading and three tapes')
t.True(rows[1] =~# '20260930-0949-external$', 'newest first')
t.True(rows[2] =~# '20260930-0054-why-basic$', 'then the next')
t.True(rows[3] =~# '20260930-0053-no-why$', 'the oldest last')
t.True(rows[1] =~# '2026-09-30 09:49 .* 9 .*  2  ', 'start time, operations and files of the first row')
t.Equal(false, getbufvar(b, '&modifiable'), 'read-only')
t.Equal(2, line('.'), 'the cursor is on the first tape')
t.Equal(1, maparg('<CR>', 'n', false, true).buffer, '<CR> is local to the buffer')
t.Equal(1, maparg('q', 'n', false, true).buffer, 'q is local to the buffer')

# A tape with a title has it at the end of its row, and the heading names the column (a list with none has no such column).
const titled = {tapeId: '20261010-0931-e0tq', startedAt: '2026-10-10T00:31:00.000Z', updatedAt: '2026-10-10T01:42:00.000Z', ops: 14, files: ['a.go'], title: 'docs first', why: 'wording'}
t.True(list.Line(titled) =~# '  20261010-0931-e0tq  docs first$', 'the title ends the row')
var plain = copy(titled)
remove(plain, 'title')
t.Equal(list.Line(plain), list.Line(extendnew(titled, {title: ''})), 'no title: the row is as it was')
t.True(list.Line(plain) =~# '20261010-0931-e0tq$', 'and it ends with the tape ID')
t.True(list.Header(true) =~# 'テープ  \+  題$', 'the heading names the title column')
t.Equal(list.Header(), '開始時刻          更新時刻            操作  ファイル  テープ', 'no title column without a title')

# <CR> on a row opens that tape.
cursor(3, 1)
execute "normal \<CR>"
t.WaitFor((): bool => replay.Active(), 'the tape to open')
t.Equal('20260930-0054-why-basic', replay.Session().tape, 'the tape of the row')
t.Equal(false, list.Active(), 'the list is closed')
ui.Close()
t.Equal(1, tabpagenr('$'), 'no tab is left')

# q closes the list.
ui.Open('')
t.WaitFor((): bool => list.Active(), 'the list again')
execute "normal q"
t.Equal(false, list.Active(), 'q closes it')
t.Equal(1, tabpagenr('$'), 'and its tab')
t.Equal([], filter(mapnew(getbufinfo(), (_, x) => x.name), (_, n) => n =~# '^srwr://'), 'no buffer is left')

# no tapes
server.Stop()
t.WaitFor((): bool => !server.Running(), 'the server to end')
t.Workspace(false)
ui.Open('')
t.WaitFor((): bool => list.Active(), 'the empty list')
t.Equal(['テープがありません（この作業場の .srwr/tapes/）'], getbufline(list.Buf(), 1, '$'), 'no tapes')
list.Close()

t.Finish()
