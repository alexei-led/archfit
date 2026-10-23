import { spawnSync } from 'node:child_process';
import { mkdir, readFile, readdir, rm, stat, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import GithubSlugger from 'github-slugger';
import MarkdownIt from 'markdown-it';
import { lint as lintMarkdown } from 'markdownlint/promise';
import YAML from 'yaml';

const here = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(here, '../../..');
const cacheDir = path.join(repoRoot, '.archfit-cache/docs');
const markdown = new MarkdownIt({ html: true });

function fail(message, errors) {
  errors.push(message);
}

function headingText(token) {
  return (token.children ?? []).map((child) => {
    if (child.type === 'text' || child.type === 'code_inline' || child.type === 'image') return child.content;
    return '';
  }).join('');
}

function linksAndAnchors(tokens, sourcePath, errors) {
  const anchors = new Set();
  const slugger = new GithubSlugger();
  const links = [];

  for (let i = 0; i < tokens.length; i += 1) {
    const token = tokens[i];
    if (token.type === 'heading_open') {
      const inline = tokens[i + 1];
      const slug = slugger.slug(headingText(inline));
      if (anchors.has(slug)) fail(`${sourcePath}: duplicate heading anchor #${slug}`, errors);
      anchors.add(slug);
    }
    if (token.type === 'inline') {
      for (const child of token.children ?? []) {
        if (child.type === 'link_open') {
          const href = child.attrGet('href');
          if (href) links.push({ href, line: token.map?.[0] + 1 || 1 });
        }
        if (child.type === 'image') {
          const src = child.attrGet('src');
          if (src) links.push({ href: src, line: token.map?.[0] + 1 || 1 });
        }
        if (child.type === 'html_inline') addHtmlAnchors(child.content, anchors);
      }
    }
    if (token.type === 'html_block') addHtmlAnchors(token.content, anchors);
  }
  return { anchors, links };
}

function addHtmlAnchors(source, anchors) {
  for (const match of source.matchAll(/\b(?:id|name)\s*=\s*(?:"([^"]+)"|'([^']+)'|([^\s>]+))/gi)) {
    anchors.add(match[1] ?? match[2] ?? match[3]);
  }
}

function checkYaml(source, label, errors) {
  const document = YAML.parseDocument(source, { prettyErrors: true, uniqueKeys: true, strict: true });
  if (document.errors.length) {
    for (const error of document.errors) fail(`${label}: ${error.message}`, errors);
  }
}

function checkJson(source, label, errors) {
  try {
    JSON.parse(source);
  } catch (error) {
    fail(`${label}: ${error.message}`, errors);
  }
}

function parseExamples(tokens, sourcePath, errors) {
  for (const token of tokens) {
    if (token.type !== 'fence') continue;
    const info = token.info.trim().toLowerCase().split(/\s+/, 1)[0];
    const label = `${sourcePath}:${token.map?.[0] + 1 || 1}`;
    if (info === 'yaml' || info === 'yml') checkYaml(token.content, label, errors);
    if (info === 'json') checkJson(token.content, label, errors);
  }
}

function isExternal(href) {
  return /^(?:[a-z][a-z\d+.-]*:|\/\/)/i.test(href) || href.startsWith('mailto:') || href.startsWith('data:');
}

export async function walk(dir, predicate, { optionalRoot = false, recursive = true, readDirectory = readdir } = {}) {
  let entries;
  try {
    entries = await readDirectory(dir, { withFileTypes: true });
  } catch (error) {
    if (optionalRoot && error.code === 'ENOENT') return [];
    throw error;
  }
  const files = [];
  for (const entry of entries) {
    const fullPath = path.join(dir, entry.name);
    if (entry.isDirectory() && recursive) {
      files.push(...await walk(fullPath, predicate, { recursive, readDirectory }));
    } else if (entry.isFile() && predicate(fullPath)) {
      files.push(fullPath);
    }
  }
  return files;
}

export async function documentationFiles(root = repoRoot) {
  const selected = [
    'README.md', 'CONTRIBUTING.md', 'CLAUDE.md', 'docs/plans/current.md',
  ].map((name) => path.join(root, name));
  selected.push(...await walk(path.join(root, 'docs'), (file) => file.endsWith('.md'), { recursive: false }));
  selected.push(...await walk(path.join(root, 'skills'), (file) => file.endsWith('.md')));
  return [...new Set(selected)];
}

export async function validateMarkdown(source, sourcePath, root = repoRoot) {
  const errors = [];
  const tokens = markdown.parse(source, {});
  parseExamples(tokens, sourcePath, errors);
  const { anchors, links } = linksAndAnchors(tokens, sourcePath, errors);
  await checkLinksFromRoot(root, sourcePath, links, anchors, errors);
  return errors;
}

export async function mermaidBlocks(files) {
  const diagrams = [];
  for (const file of files) {
    const source = await readFile(file, 'utf8');
    for (const [index, token] of markdown.parse(source, {}).entries()) {
      if (token.type === 'fence' && token.info.trim().toLowerCase().split(/\s+/, 1)[0] === 'mermaid') {
        diagrams.push({ file, index, source: token.content });
      }
    }
  }
  return diagrams;
}

export async function markdownFindings(strings, config) {
  const results = await lintMarkdown({ strings, config });
  return Object.entries(results).flatMap(([file, findings]) => findings.map((finding) =>
    `${file}:${finding.lineNumber}: ${finding.ruleNames.join('/')} ${finding.ruleDescription}`));
}

async function renderMermaid(files) {
  const diagrams = await mermaidBlocks(files);
  if (!diagrams.length) return { count: 0, outputs: [] };

  await rm(cacheDir, { recursive: true, force: true });
  await mkdir(cacheDir, { recursive: true });
  const outputs = [];
  for (const [index, diagram] of diagrams.entries()) {
    const stem = path.basename(diagram.file, '.md').replace(/[^a-z0-9_-]/gi, '-') + `-${index + 1}`;
    const input = path.join(cacheDir, `${stem}.mmd`);
    const output = path.join(cacheDir, `${stem}.svg`);
    await writeFile(input, diagram.source);
    const result = spawnSync(path.join(here, 'node_modules/.bin/mmdc'), ['-i', input, '-o', output], { cwd: repoRoot, encoding: 'utf8' });
    if (result.status !== 0) throw new Error(`Mermaid render failed for ${path.relative(repoRoot, diagram.file)}: ${(result.stderr || result.stdout).trim()}`);
    outputs.push(output);
  }
  return { count: diagrams.length, outputs };
}

export async function runCheck(root = repoRoot) {
  const files = await documentationFiles(root);
  const errors = [];
  const strings = {};
  for (const file of files) {
    const source = await readFile(file, 'utf8');
    const relative = path.relative(root, file).split(path.sep).join('/');
    strings[relative] = source;
    const tokens = markdown.parse(source, {});
    parseExamples(tokens, relative, errors);
    const { anchors, links } = linksAndAnchors(tokens, relative, errors);
    await checkLinksFromRoot(root, relative, links, anchors, errors);
  }

  for (const file of await walk(path.join(root, 'examples'), (name) => /\.ya?ml$/i.test(name), { optionalRoot: true })) {
    checkYaml(await readFile(file, 'utf8'), path.relative(root, file), errors);
  }
  const markdownConfig = JSON.parse(await readFile(path.join(root, '.markdownlint.json'), 'utf8'));
  errors.push(...await markdownFindings(strings, markdownConfig));
  if (errors.length) {
    console.error(errors.map((error) => `error: ${error}`).join('\n'));
    return 1;
  }

  const diagramResult = await renderMermaid(files);
  for (const output of diagramResult.outputs) console.log(`rendered ${path.relative(root, output)}`);
  console.log(`Documentation checks passed (${files.length} Markdown files; ${diagramResult.count} Mermaid diagrams).`);
  return 0;
}

async function checkLinksFromRoot(root, sourcePath, links, anchors, errors) {
  for (const { href, line } of links) {
    if (isExternal(href)) continue;
    let decoded;
    try { decoded = decodeURIComponent(href); } catch { fail(`${sourcePath}:${line}: malformed link escape in ${href}`, errors); continue; }
    const hashAt = decoded.indexOf('#');
    const queryAt = decoded.indexOf('?');
    const cut = [hashAt, queryAt].filter((index) => index >= 0).sort((a, b) => a - b)[0] ?? decoded.length;
    const targetPath = decoded.slice(0, cut);
    const fragment = hashAt >= 0 ? decoded.slice(hashAt + 1).split('?')[0] : '';
    const target = targetPath.startsWith('/')
      ? path.resolve(root, `.${targetPath}`)
      : path.resolve(root, targetPath ? path.dirname(sourcePath) : '.', targetPath || sourcePath);
    if (target !== root && !target.startsWith(`${root}${path.sep}`)) {
      fail(`${sourcePath}:${line}: link target escapes the repository: ${href}`, errors);
      continue;
    }
    let targetIsDirectory = false;
    try {
      if (targetPath) {
        const targetStat = await stat(target);
        if (!targetStat.isFile() && !targetStat.isDirectory()) throw new Error('unsupported target');
        targetIsDirectory = targetStat.isDirectory();
      }
    } catch {
      fail(`${sourcePath}:${line}: link target does not exist: ${href}`, errors);
      continue;
    }
    if (fragment && targetIsDirectory) {
      fail(`${sourcePath}:${line}: directory link cannot have an anchor: ${href}`, errors);
      continue;
    }
    if (!fragment || path.extname(target).toLowerCase() !== '.md') continue;
    const targetRelative = path.relative(root, target).split(path.sep).join('/');
    if (targetRelative === sourcePath) {
      if (!anchors.has(fragment)) fail(`${sourcePath}:${line}: anchor does not exist: #${fragment}`, errors);
      continue;
    }
    try {
      const targetTokens = markdown.parse(await readFile(target, 'utf8'), {});
      if (!linksAndAnchors(targetTokens, targetRelative, []).anchors.has(fragment)) fail(`${sourcePath}:${line}: anchor does not exist in ${targetRelative}: #${fragment}`, errors);
    } catch {
      fail(`${sourcePath}:${line}: cannot read link target: ${href}`, errors);
    }
  }
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try { process.exitCode = await runCheck(); }
  catch (error) { console.error(`error: ${error.message}`); process.exitCode = 1; }
}
