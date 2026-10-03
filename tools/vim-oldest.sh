#!/bin/sh
# Builds Vim 9.0.0784, the oldest Vim srwr-view.vim supports (docs/reference/vim.md), and runs `qsoku vim-test` with it.
# The build is kept and reused. SRWR_VIM_OLDEST_DIR says where it lives (the default is under the user cache; a sandbox that
# cannot write there can name another directory).
set -eu

tag=v9.0.0784
dir=${SRWR_VIM_OLDEST_DIR:-${XDG_CACHE_HOME:-$HOME/.cache}/srwr/vim-${tag#v}}

if [ ! -x "$dir/src/vim" ]; then
  rm -rf "$dir"
  mkdir -p "$(dirname "$dir")"
  git clone --quiet --filter=blob:none https://github.com/vim/vim.git "$dir"
  cd "$dir"
  git checkout --quiet "$tag"
  # Without the ncurses development package there is only libtinfo.so.6; a link named libtinfo.so is enough to link.
  libs=$dir/.libs
  mkdir -p "$libs"
  for f in /usr/lib/*/libtinfo.so.6 /lib/*/libtinfo.so.6 /usr/lib/libtinfo.so.6; do
    if [ -e "$f" ]; then
      ln -sf "$f" "$libs/libtinfo.so"
      break
    fi
  done
  LDFLAGS="-L$libs ${LDFLAGS:-}" ./configure --with-features=huge --disable-gui --without-x --disable-nls --with-tlib=tinfo >/dev/null
  make -j"$(nproc)" >/dev/null
  cd - >/dev/null
fi

"$dir/src/vim" --version | sed -n 1p
VIMRUNTIME=$dir/runtime VIM_BIN=$dir/src/vim SRWR_VIM=$dir/src/vim qsoku vim-test
