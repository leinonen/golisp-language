if exists('b:current_syntax')
  finish
endif

" Inherit the Clojure syntax — covers parens, strings, numbers,
" keywords (:foo), comments (;), defn/def/fn/let/if/when/cond/do/->/->>.
runtime! syntax/clojure.vim

" Type annotations: ^int  ^*http.Request  ^[string error]  ^(chan int)
syn match glispTypeAnnot '\^[a-zA-Z*([][a-zA-Z0-9.*\[\]() ]*'
hi def link glispTypeAnnot Type

" glisp-specific special forms not present in Clojure
syn keyword glispSpecial
  \ defstruct definterface deftest
  \ go defer chan send! recv! close! select! if-err
  \ values
hi def link glispSpecial Statement

let b:current_syntax = 'glsp'
