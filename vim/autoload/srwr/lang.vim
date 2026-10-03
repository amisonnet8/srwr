vim9script

# The language of what srwr-view.vim shows: English, or Japanese when $SRWR_LANG starts with "ja".
# It is read on every call, so a test (or a person) can change it.

export def Ja(): bool
  return tolower(trim($SRWR_LANG)) =~# '^ja'
enddef

# Pick is the Japanese text when the screen is Japanese, the English text otherwise.
export def Pick(en: string, ja: string): string
  return Ja() ? ja : en
enddef
