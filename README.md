# review

Review a diff in your browser like a GitHub PR, check off each change as you go, and pick up where you left off after the
next change. One static binary, no server.

```sh
git diff main | review
```

## Features

- **Checkmarks that survive re-renders.** Tick each block of changed lines, or mark a whole file Viewed. Render again
  after more edits and the blocks you already reviewed stay checked; only the ones that changed come back.
- **Unified or split view**, with changed words highlighted and a resizable split.
- **Syntax highlighting** for common languages, built in.
- **Light and dark**, following your OS.
- **Any diff**: `git diff`, `git show`, `git format-patch` series, plain `diff -u`.
- **Nothing to run**: pages are static HTML files in `~/.cache/review`; checks live in the browser's localStorage.

## Install

```sh
go install github.com/Donnype/review@latest
```

Needs Go 1.27+ and `~/go/bin` on your `PATH`. Prebuilt Linux, macOS and Windows binaries are attached to each
[CI run](https://github.com/Donnype/review/actions).

## Usage

| Command                    | What it does                                         |
|----------------------------|------------------------------------------------------|
| `git diff \| review`       | Render stdin, named after the current branch         |
| `review <file.patch>`      | Render a patch file                                  |
| `review list`              | List rendered pages, newest first                    |
| `review open [name\|n]`    | Reopen by list number or id prefix (default: newest) |
| `review rm <name\|n>...`   | Move pages to the trash                              |
| `review restore <path>...` | Restore trashed pages                                |
| `review completion`        | Print zsh completion: `source <(review completion)`  |

Set `$BROWSER` to pick the browser (`BROWSER=firefox review …`).

## How it works

```
diff ──► review ──► ~/.cache/review/<branch>-<hash>.html ──► browser
```

The binary embeds the diff into a self-contained HTML page and opens it; parsing and rendering happen in the page.
Checks carry over between pages because Chrome shares one localStorage across `file://` pages.

## Development

```sh
go install .          # page.html and highlight.min.js are embedded: rebuild after editing them
go vet . && gofmt -l .
node parse_test.mjs   # tests the parser shipped in page.html
```
