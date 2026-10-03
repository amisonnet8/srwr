vim9script
import './helpers.vim' as t
t.Setup()
import autoload 'srwr/hl.vim'

# The measured values of the previous implementation (vim/test/baseline/hl_*.json, from handoff/checklist/captured).
def Measured(theme: string): dict<any>
  return json_decode(join(readfile(t.Root() .. '/vim/test/baseline/hl_' .. theme .. '.json'), "\n"))
enddef

def Attrs(name: string): dict<any>
  var a = copy(hlget(name)[0])
  for k in ['id', 'default']
    if has_key(a, k)
      remove(a, k)
    endif
  endfor
  return a
enddef

def Check(theme: string)
  for [name, want] in items(Measured(theme))
    var w = copy(want)
    remove(w, 'id')
    t.Equal(w, Attrs(name), theme .. ' ' .. name)
  endfor
enddef

set background=dark
hl.Setup()
Check('dark')

def CheckOtherGroupsFollow()
  t.Equal('#fde3c8', hlget('SrwrReplace')[0].guibg, 'the other groups follow the background')
enddef

# 'background' changes: the groups follow, because they still hold what the plugin set.
set background=light
# OptionSet is not sent in a headless Vim run with -Es; send it the way Vim would.
doautocmd OptionSet background
Check('light')
set background=dark
doautocmd OptionSet background
Check('dark')

# A color the user set is never replaced, not at the start and not when 'background' changes.
set background=dark
highlight SrwrSelect guibg=#ff0000
hl.Setup()
t.Equal('#ff0000', hlget('SrwrSelect')[0].guibg, 'a color set by the user stays')
set background=light
doautocmd OptionSet background
t.Equal('#ff0000', hlget('SrwrSelect')[0].guibg, 'and stays when background changes')
CheckOtherGroupsFollow()

# The text property types exist and use the groups.
for [name, group] in [['select', 'SrwrSelect'], ['replace', 'SrwrReplace'], ['why_select', 'SrwrWhySelect'], ['why_replace', 'SrwrWhyReplace'],
    ['current', 'SrwrCurrent'], ['dot_select', 'SrwrDotSelect'], ['dot_replace', 'SrwrDotReplace'], ['dot_external', 'SrwrDotExternal']]
  t.Equal(group, prop_type_get('srwr_' .. name).highlight, 'prop type ' .. name)
endfor

t.Finish()
