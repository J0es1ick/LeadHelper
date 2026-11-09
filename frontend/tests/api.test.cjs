const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs'), vm = require('node:vm'), ts = require('typescript');
function load(fetch) {
  const m = { exports: {} }, timers = new Map(); let id = 0;
  vm.runInNewContext(ts.transpileModule(fs.readFileSync('src/shared/api.ts', 'utf8'), {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
  }).outputText, { exports: m.exports, fetch, AbortController,
    setTimeout: (fn, delay) => { timers.set(++id, { fn, delay }); return id; },
    clearTimeout: key => timers.delete(key),
  });
  return { api: m.exports.api, timers };
}
const hang = (_, { signal }) => new Promise((resolve, reject) => {
  if (signal.aborted) return reject(signal.reason);
  signal.addEventListener('abort', () => reject(signal.reason), { once: true });
});
test('stalled GET times out, clears its timer and allows a subsequent fresh read', async () => {
  let pending = true, lastOptions;
  const { api, timers } = load((url, options) => {
    lastOptions = options;
    return pending ? hang(url, options) : Promise.resolve({ ok: true, json: async () => ({ revision: 2 }) });
  });
  const first = api('/state');
  const failed = assert.rejects(first, /Проверка будет повторена автоматически/);
  assert.equal([...timers.values()][0].delay, 20000);
  [...timers.values()][0].fn(); await failed;
  assert.equal(timers.size, 0);
  pending = false;
  assert.equal((await api('/state')).revision, 2);
  assert.equal(lastOptions.cache, 'no-store');
  assert.equal(timers.size, 0);
});
test('navigation abort remains AbortError; late data cannot be mistaken for a timeout', async () => {
  const { api, timers } = load(hang), c = new AbortController();
  const first = api('/workspace/bitrix', 'GET', undefined, c.signal);
  const failed = assert.rejects(first, { name: 'AbortError' });
  c.abort(); await failed;
  assert.equal(timers.size, 0);
});
