if exists('b:current_syntax')
  finish
endif

" Allow - and ! in keyword characters so syn keyword matches hyphenated
" forms (if-err, let-or, send!, etc.) and clojure runtime picks them up too.
setlocal iskeyword+=!,?,/,-,>

" Inherit the Clojure syntax — covers parens, strings, numbers,
" keywords (:foo), comments (;), defn/def/fn/let/if/when/cond/do/->/->>.
runtime! syntax/clojure.vim

" Type annotations — three forms:
"   ^int  ^*web/Request  ^error       (simple / pointer / qualified)
"   ^[string error]                   (multi-return — match to closing ])
"   ^(chan int)                        (channel — match to closing ))
syn match glispTypeAnnot '\^\*\?[a-zA-Z][a-zA-Z0-9./]*'
syn match glispTypeAnnot '\^\[[^\]]*\]'
syn match glispTypeAnnot '\^([^)]*)'
hi def link glispTypeAnnot Type

" Core special forms (may not be in clojure.vim if runtime is absent)
syn keyword glispCore
  \ def defn fn let if when cond do ns loop recur
  \ if-let when-let
hi def link glispCore Statement

" glisp-specific special forms not in Clojure
syn keyword glispSpecial
  \ defstruct definterface defmethod deftest
  \ go defer chan send! recv! close! select! if-err
  \ let-or switch as return values
hi def link glispSpecial Statement

" Add glisp groups to Clojure's top-level cluster so they match inside parens
syn cluster clojureTop add=glispCore,glispSpecial,glispTypeAnnot

let b:current_syntax = 'glsp'
