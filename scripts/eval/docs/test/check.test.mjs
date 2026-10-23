import assert from 'node:assert/strict';
import { mkdtemp, mkdir, rm, writeFile } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import test from 'node:test';
import { markdownFindings, mermaidBlocks, validateMarkdown, walk } from '../check.mjs';

async function withRoot(run) {
  const root = await mkdtemp(path.join(os.tmpdir(), 'archfit-docs-'));
  try { await run(root); } finally { await rm(root, { recursive: true, force: true }); }
}

test('reports malformed YAML and JSON examples', async () => {
  await withRoot(async (root) => {
    const errors = await validateMarkdown('```yaml\na: [\n```\n\n```json\n{bad}\n```', 'README.md', root);
    assert.equal(errors.length, 2);
    assert.match(errors.join('\n'), /README\.md:1:/);
  });
});

test('reports missing relative files and anchors', async () => {
  await withRoot(async (root) => {
    await mkdir(path.join(root, 'docs'));
    await writeFile(path.join(root, 'docs', 'target.md'), '# Present\n');
    const errors = await validateMarkdown('[file](missing.md) [anchor](target.md#absent) ![image](missing.png) [outside](../../../tmp/private)', 'docs/source.md', root);
    assert.equal(errors.length, 4);
    assert.match(errors.join('\n'), /link target does not exist/);
    assert.match(errors.join('\n'), /anchor does not exist/);
    assert.match(errors.join('\n'), /link target escapes the repository/);
  });
});

test('uses unique GitHub heading slugs, including inline code', async () => {
  await withRoot(async (root) => {
    const errors = await validateMarkdown('# API `<x>`\n\n[valid](#api-x) [second](#api-x-1)\n\n# API `<x>`', 'README.md', root);
    assert.deepEqual(errors, []);
  });
});

test('resolves self anchors from nested Markdown files and accepts folder links', async () => {
  await withRoot(async (root) => {
    await mkdir(path.join(root, 'docs', 'nested'), { recursive: true });
    await mkdir(path.join(root, 'docs', 'reference'));
    const source = '# Page\n\n<a id="custom-anchor"></a>\n\n[self](#page) [html](#custom-anchor) [folder](../reference/)';
    const errors = await validateMarkdown(source, 'docs/nested/page.md', root);
    assert.deepEqual(errors, []);
  });
});

test('does not treat links inside code as links', async () => {
  await withRoot(async (root) => {
    const errors = await validateMarkdown('`[inline](missing.md)`\n\n```md\n[block](missing.md)\n```', 'README.md', root);
    assert.deepEqual(errors, []);
  });
});

test('finds no Mermaid blocks in ordinary Markdown', async () => {
  await withRoot(async (root) => {
    const file = path.join(root, 'README.md');
    await writeFile(file, '# No diagrams\n\n```text\nflowchart\n```\n');
    assert.deepEqual(await mermaidBlocks([file]), []);
  });
});

test('reports Markdown style violations', async () => {
  const findings = await markdownFindings({ 'docs/bad.md': '#Bad\n' }, {});
  assert.ok(findings.some((finding) => finding.includes('MD018')));
});

test('fails when a documentation directory cannot be read', async () => {
  await withRoot(async (root) => {
    const denied = Object.assign(new Error('permission denied'), { code: 'EACCES' });
    await assert.rejects(
      walk(root, () => true, { readDirectory: async () => { throw denied; } }),
      /permission denied/,
    );
  });
});
