import { cp, rm, readFile, writeFile } from 'node:fs/promises';

// Go embeds webui/static, not frontend/dist. Keep every frontend build deployable.
const source = new URL('../dist/', import.meta.url);
const target = new URL('../../webui/static/', import.meta.url);
const index = new URL('index.html', source);
await writeFile(index, (await readFile(index, 'utf8')).replace(/[ \t]+$/gm, ''));
await rm(target, { recursive: true, force: true });
await cp(source, target, { recursive: true });
console.log('Updated webui/static for the Go embedded frontend.');
