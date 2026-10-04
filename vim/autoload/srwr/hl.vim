vim9script

# The highlight groups (docs/reference/vim.md 2). They are defined with `default`, so a vimrc can override them.
# `default` never replaces a group that has settings, so when 'background' changes the groups that still hold what
# this script set are cleared first.

var applied: dict<dict<any>> = {}

def Attrs(name: string): dict<any>
  const l = hlget(name)
  if empty(l)
    return {}
  endif
  var a: dict<any> = copy(l[0])
  for k in ['id', 'name', 'default', 'cleared', 'linksto']
    if has_key(a, k)
      remove(a, k)
    endif
  endfor
  return a
enddef

def Define(name: string, dark: dict<any>, light: dict<any>)
  const want: dict<any> = &background ==# 'light' ? light : dark
  var current = Attrs(name)
  if has_key(applied, name) && current == applied[name]
    # Still what this script set: it may change with 'background'.
    hlset([{name: name, cleared: true}])
    current = {}
  endif
  if empty(current)
    hlset([extend({name: name, default: true}, want)])
    applied[name] = Attrs(name)
  endif
  # Anything else was set by the user or a colorscheme: it stays.
enddef

# Setup defines the groups and the text property types; call it again after a colorscheme change.
export def Setup()
  augroup srwr_hl
    autocmd!
    autocmd ColorScheme * call Setup()
    autocmd OptionSet background call Setup()
  augroup END
  # The range: the same hue as the why line, lighter.
  Define('SrwrSelect', {guibg: '#1d3a5c', ctermbg: '24'}, {guibg: '#cfe3fb', ctermbg: '153'})
  Define('SrwrReplace', {guibg: '#583c27', ctermbg: '94'}, {guibg: '#fde3c8', ctermbg: '223'})
  # The why line: white bold text on the saturated color, whatever the colorscheme is.
  const sel = {cterm: {bold: true}, gui: {bold: true}, ctermfg: '15', ctermbg: '25', guifg: '#ffffff', guibg: '#0b61a4'}
  const rep = {cterm: {bold: true}, gui: {bold: true}, ctermfg: '15', ctermbg: '130', guifg: '#ffffff', guibg: '#b45f06'}
  Define('SrwrWhySelect', sel, sel)
  Define('SrwrWhyReplace', rep, rep)
  const fail = {cterm: {bold: true}, gui: {bold: true}, ctermfg: '15', ctermbg: '196', guifg: '#ffffff', guibg: '#d50000'}
  Define('SrwrWhyFailure', fail, fail)
  # The operation list: the current line, the dot of each kind, a button that cannot be used now.
  Define('SrwrCurrent', {guibg: '#3a3d41', ctermbg: '238'}, {guibg: '#e4e6f1', ctermbg: '254'})
  Define('SrwrDotSelect', {guifg: '#4aa3ff', ctermfg: '39'}, {guifg: '#0b61a4', ctermfg: '25'})
  Define('SrwrDotReplace', {guifg: '#f0883e', ctermfg: '208'}, {guifg: '#b45f06', ctermfg: '130'})
  Define('SrwrDotExternal', {guifg: '#b180d7', ctermfg: '140'}, {guifg: '#652d90', ctermfg: '54'})
  Define('SrwrDotFailure', {guifg: '#ff3b30', ctermfg: '196'}, {guifg: '#d50000', ctermfg: '160'})
  Define('SrwrDim', {guifg: '#808080', ctermfg: '244'}, {guifg: '#909090', ctermfg: '244'})

  for [name, group, prio] in [
      ['select', 'SrwrSelect', 10], ['replace', 'SrwrReplace', 10],
      ['why_select', 'SrwrWhySelect', 30], ['why_replace', 'SrwrWhyReplace', 30], ['why_failure', 'SrwrWhyFailure', 30],
      ['current', 'SrwrCurrent', 10], ['num', 'LineNr', 5], ['dot_select', 'SrwrDotSelect', 20], ['dot_replace', 'SrwrDotReplace', 20],
      ['dot_external', 'SrwrDotExternal', 20], ['dot_failure', 'SrwrDotFailure', 20]]
    if empty(prop_type_get('srwr_' .. name))
      prop_type_add('srwr_' .. name, {highlight: group, priority: prio})
    endif
  endfor
enddef
