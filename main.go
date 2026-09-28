// review renders a unified diff as a code review page and opens it in the browser.
package main

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"
)

//go:embed page.html
var page string

// Written once next to the pages, which load it by relative path; highlight.js 11.11.1 common build (BSD-3).
//
//go:embed highlight.min.js
var highlightJS []byte

const usage = `usage:
  git diff | review         render a diff from stdin, named after the git branch
  review <file.patch>       render a patch file, named after the file
  review list               list rendered diffs, newest first
  review open [name|n]      reopen a diff by unique name prefix or list number (default: newest)
  review rm <name|n>...     move rendered diffs to the trash (prints where, for restore)
  review restore <path>...  move trashed pages back
  review completion         print zsh completion; add 'source <(review completion)' to ~/.zshrc`

// Completes subcommands and patch files, then page ids (newest first) after "open".
const completion = `_review() {
  if (( CURRENT == 2 )); then compadd list open rm restore completion; _files
  elif [[ $words[2] == (open|rm) ]]; then compadd -V ids -- ${(f)"$(review list | awk '{print $NF}')"}; fi
}
compdef _review review`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "review:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	dir, err := cacheDir()
	if err != nil {
		return err
	}
	if len(args) == 0 {
		if fi, _ := os.Stdin.Stat(); fi.Mode()&os.ModeCharDevice != 0 {
			return fmt.Errorf("no diff on stdin\n%s", usage)
		}
		return render(dir, os.Stdin, branch())
	}
	switch args[0] {
	case "-h", "--help", "help":
		fmt.Println(usage)
		return nil
	case "completion":
		fmt.Println(completion)
		return nil
	case "rm":
		if len(args) < 2 {
			return fmt.Errorf("rm needs a name or list number")
		}
		trash, err := trashDir()
		if err != nil {
			return err
		}
		for _, q := range args[1:] {
			src, err := find(dir, q)
			if err != nil {
				return err
			}
			dst := filepath.Join(trash, filepath.Base(src))
			if err := move(src, dst); err != nil {
				return err
			}
			fmt.Println(dst)
		}
		return nil
	case "restore":
		for _, src := range args[1:] {
			if err := move(src, filepath.Join(dir, filepath.Base(src))); err != nil {
				return err
			}
		}
		return nil
	case "list":
		return list(dir)
	case "open":
		query := ""
		if len(args) > 1 {
			query = args[1]
		}
		path, err := find(dir, query)
		if err != nil {
			return err
		}
		return open(path)
	}
	f, err := os.Open(args[0])
	if err != nil {
		return err
	}
	defer f.Close()
	base := filepath.Base(args[0])
	return render(dir, f, strings.TrimSuffix(base, filepath.Ext(base)))
}

// branch names a stdin diff after the current git branch, falling back to the directory name.
func branch() string {
	if out, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output(); err == nil {
		if b := strings.TrimSpace(string(out)); b != "" && b != "HEAD" {
			return b
		}
	}
	cwd, _ := os.Getwd()
	return filepath.Base(cwd)
}

func cacheDir() (string, error) {
	base := os.Getenv("XDG_CACHE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".cache")
	}
	dir := filepath.Join(base, "review")
	return dir, os.MkdirAll(dir, 0o755)
}

// trashDir is the macOS Trash, else a temp dir the OS cleans up eventually. The XDG Trash is skipped because
// its entries need .trashinfo sidecars to be listed or restored by file managers.
func trashDir() (string, error) {
	if runtime.GOOS == "darwin" {
		home, err := os.UserHomeDir()
		return filepath.Join(home, ".Trash"), err
	}
	dir := filepath.Join(os.TempDir(), "review-trash")
	return dir, os.MkdirAll(dir, 0o755)
}

// move renames, falling back to copy+remove when src and dst are on different filesystems (e.g. tmpfs /tmp).
func move(src, dst string) error {
	if os.Rename(src, dst) == nil {
		return nil
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return err
	}
	return os.Remove(src)
}

var unsafeChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// render writes the page to <dir>/<name>-<content-hash>.html, so re-rendering an identical diff reuses its page.
func render(dir string, r io.Reader, name string) error {
	diff, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(diff)) == "" {
		return fmt.Errorf("empty diff")
	}
	sum := sha256.Sum256(diff)
	id := unsafeChars.ReplaceAllString(name, "-") + "-" + hex.EncodeToString(sum[:])[:6]
	// json.Marshal escapes <, > and &, so the diff can't break out of its <script> tag.
	data, _ := json.Marshal(string(diff))
	out := strings.NewReplacer(
		"{{TITLE}}", html.EscapeString(name+" · "+time.Now().Format("Jan 2 15:04")),
		"{{DIFF}}", string(data),
	).Replace(page)
	js := filepath.Join(dir, "highlight.min.js")
	if fi, err := os.Stat(js); err != nil || fi.Size() != int64(len(highlightJS)) {
		if err := os.WriteFile(js, highlightJS, 0o644); err != nil {
			return err
		}
	}
	path := filepath.Join(dir, id+".html")
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		return err
	}
	fmt.Println(id)
	return open(path)
}

type entry struct {
	id, path string
	mod      time.Time
}

// name is the id without its content-hash suffix: re-renders of one branch share a name.
func (e entry) name() string {
	if i := strings.LastIndexByte(e.id, '-'); i > 0 {
		return e.id[:i]
	}
	return e.id
}

func entries(dir string) ([]entry, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.html"))
	if err != nil {
		return nil, err
	}
	var es []entry
	for _, p := range paths {
		if fi, err := os.Stat(p); err == nil {
			es = append(es, entry{strings.TrimSuffix(filepath.Base(p), ".html"), p, fi.ModTime()})
		}
	}
	slices.SortFunc(es, func(a, b entry) int { return b.mod.Compare(a.mod) })
	return es, nil
}

func list(dir string) error {
	es, err := entries(dir)
	if err != nil {
		return err
	}
	for i, e := range es {
		fmt.Printf("%3d  %s  %s\n", i+1, e.mod.Format("Mon Jan _2 15:04"), e.id)
	}
	return nil
}

// find resolves a list number, an exact id, or a prefix. A prefix matching several renders of the same name
// picks the newest; matching different names is ambiguous.
func find(dir, query string) (string, error) {
	es, err := entries(dir)
	if err != nil {
		return "", err
	}
	if n, err := strconv.Atoi(query); err == nil && n >= 1 && n <= len(es) {
		return es[n-1].path, nil
	}
	var hits []entry
	for _, e := range es {
		if e.id == query {
			return e.path, nil
		}
		if strings.HasPrefix(e.id, query) {
			hits = append(hits, e)
		}
	}
	if len(hits) == 0 {
		return "", fmt.Errorf("no rendered diff matching %q in %s", query, dir)
	}
	var names []string
	for _, h := range hits {
		if !slices.Contains(names, h.name()) {
			names = append(names, h.name())
		}
	}
	if len(names) > 1 {
		return "", fmt.Errorf("%q is ambiguous: %s", query, strings.Join(names, ", "))
	}
	return hits[0].path, nil
}

// open uses $BROWSER when set (CI sets it to echo), else the platform opener.
func open(path string) error {
	if browser := os.Getenv("BROWSER"); browser != "" {
		cmd := exec.Command(browser, path)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		return cmd.Run()
	}
	cmd := "xdg-open"
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", path).Start()
	}
	return exec.Command(cmd, path).Start()
}
