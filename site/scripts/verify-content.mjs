import { readFile, readdir } from "node:fs/promises";
import { existsSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { DOC_META } from "../src/content/nav.en.js";

const site = dirname(dirname(fileURLToPath(import.meta.url)));
const docs = join(site, "docs");
const content = join(docs, "content");
const englishContent = join(content, "en");

function fail(message) {
  console.error(`content check failed: ${message}`);
  process.exitCode = 1;
}

function parse(raw) {
  const match = /^---\r?\n([\s\S]*?)\r?\n---\r?\n?/.exec(raw);
  const meta = {};
  if (match) {
    for (const line of match[1].split(/\r?\n/)) {
      const pair = /^([A-Za-z0-9_-]+)\s*:\s*(.*)$/.exec(line);
      if (pair) meta[pair[1]] = pair[2].trim().replace(/^["']|["']$/g, "");
    }
  }
  return { meta, body: match ? raw.slice(match[0].length) : raw };
}

function countHeadings(body) {
  return [...body.matchAll(/^#{1,6}\s+/gm)].length;
}

function countFences(body) {
  return [...body.matchAll(/^```/gm)].length;
}

function countCallouts(body) {
  return [...body.matchAll(/^> \[!([A-Z]+)\]/gm)].length;
}

function linkedFiles(body) {
  return [...body.matchAll(/\]\(([^)\s]+\.md)(?:#[^)]*)?\)/g)].map((match) => match[1].split("/").pop());
}

const manifest = JSON.parse(await readFile(join(docs, "manifest.json"), "utf8"));
const sections = Array.isArray(manifest.sections) ? manifest.sections : [];
if (sections.length === 0) fail("manifest has no sections");

const seen = new Set();
const items = [];

for (const section of sections) {
  if (!section.title) fail("a section is missing a title");
  for (const item of section.items || []) {
    if (!item.slug) fail("a document is missing a slug");
    if (seen.has(item.slug)) fail(`duplicate slug: ${item.slug}`);
    seen.add(item.slug);
    items.push({ ...item, section: section.title });
  }
}

if (items.length === 0) fail("manifest has no documents");

const fileNames = new Set((await readdir(content)).filter((name) => name.endsWith(".md")));
const englishFiles = new Set((await readdir(englishContent)).filter((name) => name.endsWith(".md")));

for (const item of items) {
  const fileName = item.file.replace(/^content\//, "");
  if (!fileNames.has(fileName)) {
    fail(`${item.slug}: missing ${item.file}`);
    continue;
  }

  const raw = await readFile(join(content, fileName), "utf8");
  const chinese = parse(raw);
  if (!chinese.meta.title) fail(`${item.slug}: missing Chinese frontmatter title`);
  if (chinese.body.replace(/```[\s\S]*?```/g, "").length < 40 && chinese.meta.status !== "stub") {
    fail(`${item.slug}: Chinese document body is empty`);
  }
  for (const target of linkedFiles(chinese.body)) {
    if (!fileNames.has(target)) fail(`${item.slug}: broken Chinese link to ${target}`);
  }

  if (!englishFiles.has(fileName)) {
    fail(`${item.slug}: missing English mirror content/en/${fileName}`);
    continue;
  }

  const englishRaw = await readFile(join(englishContent, fileName), "utf8");
  const english = parse(englishRaw);
  if (!english.meta.title) fail(`${item.slug}: missing English frontmatter title`);
  if (english.body.replace(/```[\s\S]*?```/g, "").length < 40 && english.meta.status !== "stub") {
    fail(`${item.slug}: English document body is empty`);
  }
  if (/[\u4e00-\u9fff]/.test(englishRaw)) {
    fail(`${item.slug}: Chinese characters remain in the English document`);
  }
  if (english.meta.status !== chinese.meta.status) fail(`${item.slug}: status differs between languages`);
  if (english.meta.updated !== chinese.meta.updated) fail(`${item.slug}: updated date differs between languages`);

  const expected = DOC_META[item.slug];
  if (expected) {
    if (english.meta.title !== expected.title) {
      fail(`${item.slug}: English title "${english.meta.title}" does not match nav.en.js`);
    }
    if (english.meta.description !== expected.description) {
      fail(`${item.slug}: English description does not match nav.en.js`);
    }
  }

  if (countHeadings(english.body) !== countHeadings(chinese.body)) {
    fail(`${item.slug}: heading count differs between languages`);
  }
  if (countFences(english.body) !== countFences(chinese.body)) {
    fail(`${item.slug}: fenced block count differs between languages`);
  }
  if (countCallouts(english.body) !== countCallouts(chinese.body)) {
    fail(`${item.slug}: callout count differs between languages`);
  }
  for (const target of linkedFiles(english.body)) {
    if (!englishFiles.has(target)) fail(`${item.slug}: broken English link to ${target}`);
  }
}

if (!existsSync(join(docs, "manifest.json"))) fail("manifest.json is missing");

if (!process.exitCode) {
  console.log(
    `content check: ${items.length} pages in ${sections.length} sections · Chinese + English mirrors · all files and links resolve`
  );
}
