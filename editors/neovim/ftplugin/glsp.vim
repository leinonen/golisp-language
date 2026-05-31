setlocal commentstring=;\ %s

" Format via `glisp fmt -` (stdin -> stdout)
setlocal formatprg=glisp\ fmt\ -

" Format whole buffer, preserving cursor/window position.
function! s:GlispFormat() abort
  if !executable('glisp')
    echohl WarningMsg | echom 'glisp not found in PATH' | echohl None
    return
  endif
  let l:view = winsaveview()
  let l:lines = getline(1, '$')
  let l:out = systemlist('glisp fmt -', l:lines)
  if v:shell_error != 0
    echohl ErrorMsg | echom join(l:out, ' ') | echohl None
    return
  endif
  if l:out !=# l:lines
    silent! call deletebufline('%', 1, '$')
    call setline(1, l:out)
  endif
  call winrestview(l:view)
endfunction

command! -buffer GlispFormat call s:GlispFormat()
nnoremap <buffer> <leader>f <Cmd>GlispFormat<CR>

" Format on save
augroup glisp_format
  autocmd! * <buffer>
  autocmd BufWritePre <buffer> call s:GlispFormat()
augroup END
