#!/usr/bin/env node
// Snapshot selected nof1.ai upstream API payloads to local JSON files
// Usage: node scripts/snapshot-nof1.mjs

import fs from 'node:fs/promises';
import path from 'node:path';

const BASE = 'https://nof1.ai/api';

const endpoints = [
  { key: 'crypto-prices', path: '/crypto-prices' },
  { key: 'positions', path: '/positions?limit=5000' },
  { key: 'trades', path: '/trades' },
  { key: 'account-totals', path: '/account-totals' },
  { key: 'since-inception-values', path: '/since-inception-values' },
  { key: 'leaderboard', path: '/leaderboard' },
  { key: 'analytics', path: '/analytics' },
  { key: 'conversations', path: '/conversations' },
];

async function main() {
  const ts = new Date().toISOString().replace(/[:]/g, '').replace(/\..+/, 'Z');
  const dir = path.join(process.cwd(), 'snapshots', 'nof1', ts);
  await fs.mkdir(dir, { recursive: true });

  const summary = { base: BASE, timestamp: ts, files: [] };

  for (const ep of endpoints) {
    const url = `${BASE}${ep.path}`;
    process.stdout.write(`Fetching ${url} ... `);
    try {
      const res = await fetch(url, { headers: { Accept: 'application/json' }, cache: 'no-store' });
      if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
      const text = await res.text();
      // Keep original text to avoid JSON reformatting; still validate JSON
      try { JSON.parse(text); } catch (e) {
        console.warn(`\nWarning: ${ep.key} response is not valid JSON: ${e}`);
      }
      const f = path.join(dir, `${ep.key}.json`);
      await fs.writeFile(f, text);
      summary.files.push({ key: ep.key, path: `snapshots/nof1/${ts}/${ep.key}.json`, url });
      process.stdout.write('saved\n');
    } catch (err) {
      process.stdout.write(`failed: ${err}\n`);
      summary.files.push({ key: ep.key, error: String(err), url });
    }
  }

  await fs.writeFile(path.join(dir, 'index.json'), JSON.stringify(summary, null, 2));
  console.log(`\nSnapshot complete -> ${dir}`);
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});

