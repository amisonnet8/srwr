vim9script

# The only setting is where the srwr binary is. Everything else about the look is fixed.

# Path is g:srwr_path, or "srwr" (searched in PATH).
export def Path(): string
  return get(g:, 'srwr_path', 'srwr')
enddef

# Options for the server's initialize request: diff frames are always on.
export def ServerOptions(): dict<any>
  return {diffFrames: true}
enddef
