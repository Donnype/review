// Checks the diff parser embedded in page.html: node parse_test.mjs
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';

const html = readFileSync(new URL('page.html', import.meta.url), 'utf8');
const src = html.split('<script>\n')[1].split('const files = parse')[0];
const {parse, prep} = new Function(src + 'return {parse, prep};')();

const diff = `diff --git a/src/a.go b/src/a.go
index 1111111..2222222 100644
--- a/src/a.go
+++ b/src/a.go
@@ -1,3 +1,3 @@ func main() {
 keep
--- looks like a header
+new line
 keep
\\ No newline at end of file
diff --git a/old.txt b/new.txt
similarity index 100%
rename from old.txt
rename to new.txt
diff --git a/img.png b/img.png
new file mode 100644
Binary files /dev/null and b/img.png differ
diff --git a/gone.txt b/gone.txt
deleted file mode 100644
--- a/gone.txt
+++ /dev/null
@@ -1 +0,0 @@
-bye
--- x.txt\t2026-01-01 00:00:00
+++ y.txt\t2026-01-02 00:00:00
@@ -1 +1 @@
-hello world
+hello there
`;

const files = parse(diff);
assert.deepEqual(files.map(f => [f.name, f.status]), [
  ['src/a.go', 'modified'], ['new.txt', 'renamed'], ['img.png', 'added'], ['gone.txt', 'deleted'], ['y.txt', 'renamed'],
]);
const [a, , img, gone, plain] = files;
assert.deepEqual(a.hunks[0].lines.map(l => [l.t, l.s, l.o, l.n]), [
  [' ', 'keep', 1, 1], ['-', '-- looks like a header', 2, undefined], ['+', 'new line', undefined, 2], [' ', 'keep', 3, 3],
]);
assert.equal(a.adds + a.dels, 2);
assert.ok(img.binary && !img.hunks.length && img.keys.length === 1);
assert.equal(gone.dels, 1);
prep(plain.hunks[0]);
assert.equal(plain.hunks[0].pairs[0][1].html, 'hello <mark>there</mark>');
assert.notEqual(parse(diff.replace('new line', 'other'))[0].keys[0], a.keys[0]);
assert.equal(parse(diff.replace('@@ -1,3 +1,3 @@', '@@ -9,3 +9,3 @@'))[0].keys[0], a.keys[0]);
console.log('ok');
