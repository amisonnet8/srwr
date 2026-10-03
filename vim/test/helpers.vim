vim9script

# Shared by the tests: every test is a script run by `qsoku vim-test` in a headless Vim (see .claude/rules/vim.md).

export def Root(): string
  return fnamemodify(expand('<script>:p'), ':h:h:h')
enddef

# Setup puts the plugin on 'runtimepath' and points g:srwr_path at the binary that `qsoku bin` built.
export def Setup()
  # These tests look at the Japanese texts (test_lang.vim looks at the English ones), and at times in the zone the fixed tapes
  # were recorded in (+09:00); a test that is about the zone sets $TZ itself.
  $SRWR_LANG = 'ja'
  $TZ = 'Asia/Tokyo'
  &runtimepath = Root() .. '/vim,' .. &runtimepath
  g:srwr_path = Root() .. '/bin/srwr'
  v:errors = []
enddef

# Workspace copies the fixed workspace (extension/test/fixtures/ui-check) to a new temporary directory and goes there.
export def Workspace(withTapes: bool = true): string
  const dir = tempname()
  mkdir(dir, 'p')
  system('cp -r ' .. shellescape(Root() .. '/extension/test/fixtures/ui-check') .. '/. ' .. shellescape(dir))
  if !withTapes
    system('rm -f ' .. shellescape(dir .. '/.srwr/tapes/') .. '*')
  endif
  execute 'cd ' .. fnameescape(dir)
  return dir
enddef

export def WaitFor(Cond: func(): bool, what: string, timeout: number = 8000)
  var waited = 0
  while !Cond()
    if waited >= timeout
      add(v:errors, 'timed out waiting for ' .. what)
      return
    endif
    sleep 10m
    waited += 10
  endwhile
enddef

export def Equal(want: any, got: any, what: string)
  if want != got
    add(v:errors, what .. ': want ' .. string(want) .. ', got ' .. string(got))
  endif
enddef

export def True(cond: bool, what: string)
  if !cond
    add(v:errors, what)
  endif
enddef

# Finish ends the test: qall! when nothing failed, else the errors go to stderr and the exit code is 1.
export def Finish()
  if empty(v:errors)
    qall!
  endif
  writefile(v:errors, '/dev/stderr')
  cquit 1
enddef
