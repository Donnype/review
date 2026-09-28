# review

Turn a unified diff into a review page in your browser, with per-file "Viewed" and per-hunk checkmarks
kept in the browser's localStorage. One static Go binary, no Go dependencies, no server.

```sh
git diff --staged | review
```

## Usage

| Command                    | What it does                                                                                     |
|----------------------------|--------------------------------------------------------------------------------------------------|
| `git diff \| review`       | Render the diff on stdin, named after the current git branch (or the directory outside a repo). |
| `review <file.patch>`      | Render a patch file, named after the file.                                                       |
| `review list`              | List rendered diffs, newest first: list number, date, id.                                        |
| `review open [name\|n]`    | Reopen by list number or unique id prefix (`rel` → `release-workflow-…`). Default: newest.       |
| `review rm <name\|n>...`   | Move pages to the trash, printing where each went.                                               |
| `review restore <path>...` | Move trashed pages back.                                                                         |
| `review completion`        | Print zsh completion. Add `source <(review completion)` to `~/.zshrc`.                           |

Accepted input: `git diff`/`git show`, `git format-patch` series and plain `diff -u` output.

**Name resolution in `open`/`rm`**: a number picks that row of `review list`; otherwise an exact id, then a prefix.
A prefix matching several renders of the same name (e.g. the same branch rendered twice) picks the newest; one
matching different names is an error that lists them.

**Trash**: `~/.Trash` on macOS (no Finder "Put Back"; use `review restore`), `$TMPDIR/review-trash` elsewhere, where
the OS eventually cleans it up.

**Browser**: set `$BROWSER` to open pages with a specific program (`BROWSER=firefox review …`). Without it, the
platform opener is used.

## How it works

```
stdin / file ──► review (Go) ──► ~/.cache/review/<name>-<hash>.html ──► open / xdg-open
                                 ~/.cache/review/highlight.min.js
```

The Go side does no diff parsing. It:

1. reads the whole diff and hashes it (SHA-256, first 6 hex chars);
2. builds the id `<name>-<hash>`, where the name is the git branch or file name with unsafe characters replaced by `-`.
   Re-rendering an identical diff on the same branch overwrites the same page;
3. writes `page.html` (embedded in the binary) with two placeholders filled: the title, and the raw diff as a JSON
   string inside `<script type="application/json">`. `json.Marshal` escapes `<`, `>` and `&`, so diff content can't
   break out of the script tag;
4. writes the embedded `highlight.min.js` next to the pages if it's missing or a different size, then opens the page
   with the platform opener (`open`, `xdg-open` or `rundll32`).

There is no index or database: the cache directory is the list. `list` sorts the `*.html` files there by modification
time. The cache lives at `$XDG_CACHE_HOME/review`, defaulting to `~/.cache/review` on every platform.

### The page (`page.html`)

One self-contained HTML file: inline CSS and JS, plus one relative `<script src="highlight.min.js">`.

- **Parser**: `parse()` walks the diff line by line. Inside a hunk, the `@@ -a,b +c,d @@` counts decide where the hunk
  ends, so a removed line such as `--- foo` isn't mistaken for a file header. A `diff --git` line always ends a hunk,
  which protects against hand-edited patches with wrong counts. File status (added, deleted, renamed, modified, binary)
  comes from the `---`/`+++`, `new file mode`, `deleted file mode`, `rename from/to` and `Binary files` lines.
- **Rendering**: each file is a `<section>` with a sticky header. Its body is built only the first time the file is
  expanded, so collapsed (viewed) files cost nothing. `prep()` pairs each run of deletions with the following additions
  for split view, and `mark()` highlights the changed span between a pair (common prefix/suffix). Unified and split
  views are both built from this; the choice is remembered. In split view, drag the divider or the right side's
  line numbers to resize the two sides (double-click resets it). The position is shared by all files and remembered.
- **Syntax highlighting**: highlight.js picks the language from the file extension, one line at a time. Multi-line
  constructs (block comments, template strings) can be mis-coloured past their first line. Unknown file types, or a
  missing `highlight.min.js`, fall back to plain text.
- **Theme**: CSS variables plus `color-scheme: light dark`, following the OS setting. Dark mode uses a dimmed palette
  rather than near-black.

### Checkmark state

State lives only in the browser's `localStorage`, under keys prefixed `review:`. It's keyed by **content, not by page
or line number**:

- each hunk's key is a 53-bit hash (`cyrb53`) of the file path plus the hunk's lines, excluding the `@@` header;
- a file is "Viewed" exactly when all its hunks are checked. Checking Viewed checks every hunk, and checking the last
  hunk marks the file viewed and collapses it;
- a file without hunks (binary file, pure rename, mode change) gets a single key from its paths and metadata.

As a result, re-rendering a diff after further edits keeps checks on unchanged hunks and clears them on changed ones,
and a hunk that moved because of edits above it stays checked. Two open tabs stay in sync through the `storage`
event. On `file://` pages Chrome shares one localStorage across all local files, which is what makes content keys
carry over between pages. Clearing site data, or using a private window, drops the checks.

## Files

| File               | Role                                                                     |
|--------------------|--------------------------------------------------------------------------|
| `main.go`          | CLI: subcommands, naming, cache/trash paths, writing and opening pages. |
| `page.html`        | Page template: parser, renderer, checkmark state, styles. Embedded.     |
| `highlight.min.js` | Vendored highlight.js 11.11.1, common build (BSD-3-Clause). Embedded.   |
| `parse_test.mjs`   | Node checks for the parser, word highlighting and hunk keys.            |

## Building and testing

Requires Go 1.27 (see `go.mod`). `page.html` and `highlight.min.js` are compiled into the binary with `//go:embed`,
so **edits to either only take effect after a rebuild**, and only for pages rendered after that.

```sh
go install .          # builds ~/go/bin/review (make sure it's on PATH)
go vet . && gofmt -l .
node parse_test.mjs   # runs the parser from page.html against a fixture diff; prints "ok"
```

`parse_test.mjs` pulls the `<script>` out of `page.html`, so the test runs the shipped code. It fails if the parser
changes shape (file names/statuses, line numbers, word marks, content-keyed hunk ids).

To update highlight.js, replace `highlight.min.js` with another "common" build from
`https://cdn.jsdelivr.net/gh/highlightjs/cdn-release@<version>/build/highlight.min.js` and rebuild. The next render
overwrites the cached copy, because the size check notices the change.

### CI

`.github/workflows/build.yaml` runs on pushes to `main` and on pull requests, on Linux (x64 and arm64), macOS (arm64)
and Windows (x64). Each job vets, runs `parse_test.mjs`, builds a stripped binary, and smoke-tests that binary against
a small patch: render from a file and from stdin, `list`, `open` by prefix, `rm`/`restore` and `completion`. It uses
`BROWSER=echo` so nothing opens. The binary is uploaded as the `review-<OS>-<arch>` artifact.
`.gitattributes` forces LF line endings, so Windows checkouts pass `gofmt` and the parser test.
