// GoLisp syntax highlighting for highlight.js
// Registers the 'golisp' language and re-highlights any blocks on the page.
(function () {
  if (typeof hljs === 'undefined') return;

  hljs.registerLanguage('golisp', function (hljs) {

    // Special forms: core language constructs, matched after opening paren.
    // Longer alternatives listed first to prevent prefix shadowing
    // (e.g. definterface before def, go-val before go, recv-ok! before recv!).
    const KEYWORD_RE = new RegExp(
      '(?<=\\()' +
      '(?:definterface|defmethod|defstruct|deftype|defn|def' +
        '|if-let|when-let|if-err|let-or' +
        '|go-val|recv-ok[!]|recv[!]|send[!]|close[!]|for-chan|select[!]|with-lock' +
        '|fn|let|loop|recur|if|when|cond|case|switch|do|ns|values' +
        '|and|or|not|doseq|dotimes|go|par|chan|defer|panic|recover|assert)' +
      '(?=[\\s)\\[\\]{}]|$)'
    );

    // Built-in functions from the standard library.
    // Longer/more-specific alternatives listed first.
    const BUILTIN_RE = new RegExp(
      '(?<=\\()' +
      '(?:map-vals|map-keys|reduce-kv|select-keys|rename-keys' +
        '|get-in|assoc-in|update-in|update' +
        '|group-by|frequencies|partition-by|partition|zipmap' +
        '|sort-by|min-by|max-by|min-key|max-key' +
        '|take-while|drop-while|split-at|split-with' +
        '|starts-with[?]|ends-with[?]|contains[?]|empty[?]|file-exists[?]' +
        '|every[?]|nil[?]|even[?]|odd[?]|pos[?]|neg[?]|zero[?]|blank[?]' +
        '|parse-int|parse-float' +
        '|upper-case|lower-case|capitalize' +
        '|map|filter|reduce|some' +
        '|mapcat|interleave|interpose|keep' +
        '|str|println|print|format' +
        '|get|assoc|dissoc|merge|keys|vals' +
        '|conj|count|len|first|last|rest|nth' +
        '|reverse|sort|take|drop|flatten|distinct|range|repeat' +
        '|comp|partial|complement|juxt|apply|identity|constantly|fnil' +
        '|min|max|inc|dec|abs|mod' +
        '|trim|split|join|replace|subs' +
        '|error|wrap-error' +
        '|read-file|write-file|append-file|list-dir|mkdir' +
        '|set|union|intersection|difference|into|as' +
        '|json/encode|json/decode' +
        '|http/get|http/post|http/put|http/delete|http/request' +
        '|re/match|re/find|re/replace|re/split' +
        '|log/info|log/debug|log/warn|log/error' +
        '|ctx/background|ctx/todo|ctx/with-cancel|ctx/with-timeout' +
        '|ctx/cancel[!]|ctx/value|ctx/with-value|ctx/done[?]|ctx/err)' +
      '(?=[\\s)\\[\\]{}]|$)'
    );

    return {
      name: 'GoLisp',
      aliases: ['glsp'],
      contains: [
        // Docstring (;;; ...) — must precede the general comment rule
        {
          className: 'doctag',
          begin: /;;;/,
          end: /$/,
        },
        // Line comment (; ...)
        {
          className: 'comment',
          begin: /;/,
          end: /$/,
        },
        // String literals
        hljs.QUOTE_STRING_MODE,
        // Numeric literals (int and float)
        {
          className: 'number',
          begin: /\b\d+(?:\.\d+)?(?:[eE][+-]?\d+)?\b/,
          relevance: 0,
        },
        // Boolean and nil literals
        {
          className: 'literal',
          begin: /\b(?:true|false|nil)\b/,
        },
        // Lisp keywords: :name, :some-key, :enabled?
        // Note: :- (annotated destructuring marker) doesn't start with [a-zA-Z_]
        // so it is NOT matched here — no conflict with the rule below.
        {
          className: 'symbol',
          begin: /:[a-zA-Z_][a-zA-Z0-9_\-?!]*/,
        },
        // Annotated destructuring type marker: :-
        {
          className: 'keyword',
          begin: /:-(?=\s)/,
        },
        // Return type arrow: ->
        {
          className: 'keyword',
          begin: /->/,
        },
        // Go interop: method call .Method or field read .-Field
        {
          className: 'built_in',
          begin: /\.-?[A-Z][a-zA-Z0-9]*/,
        },
        // Struct constructor literal: TypeName. followed by a map
        {
          className: 'title.class',
          begin: /\b[A-Z][a-zA-Z0-9]*\./,
        },
        // Primitive and void type keywords
        {
          className: 'type',
          begin: /\b(?:int|float64|string|bool|void|any|error)\b/,
        },
        // Qualified Go types: pkg/TypeName  (e.g. web/Request, sync/Mutex)
        {
          className: 'type',
          begin: /\b[a-z][a-z0-9]*\/[A-Z][a-zA-Z0-9]*/,
        },
        // Special forms (matched after opening paren via lookbehind)
        {
          className: 'keyword',
          begin: KEYWORD_RE,
        },
        // Built-in functions (matched after opening paren via lookbehind)
        {
          className: 'built_in',
          begin: BUILTIN_RE,
        },
        // Qualified function/constant calls: pkg/name, math/pi, time/Second, etc.
        {
          className: 'built_in',
          begin: /\b[a-z][a-z0-9]*\/[a-z][a-zA-Z0-9\-?!]*/,
        },
      ],
    };
  });

  // Re-highlight GoLisp blocks after mdBook's own pass has run.
  // mdBook bundles highlight.js v10 which uses highlightBlock(); v11 renamed it
  // to highlightElement(). We support both so this works across versions.
  // In v10, an unknown-language block is returned early (innerHTML untouched,
  // no 'hljs' class added), so no reset is needed before re-processing.
  var highlightFn = hljs.highlightElement
    ? hljs.highlightElement.bind(hljs)
    : hljs.highlightBlock.bind(hljs);

  function rehighlight() {
    document.querySelectorAll('pre code.language-golisp').forEach(highlightFn);
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', rehighlight);
  } else {
    // DOMContentLoaded already fired; yield so mdBook's own highlight pass
    // finishes first, then re-process our blocks with the registered language.
    setTimeout(rehighlight, 0);
  }
})();
