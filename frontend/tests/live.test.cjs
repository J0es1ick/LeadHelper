const { test } = require("node:test");
const assert = require("node:assert/strict");
const ts = require("typescript");
const fs = require("node:fs");
const vm = require("node:vm");

function fixture() {
  let now = 0,
    id = 0;
  const timers = new Map();
  class Target extends EventTarget {
    visibilityState = "visible";
  }
  const document = new Target(),
    window = new Target(),
    navigator = { onLine: true };
  const m = { exports: {} };
  vm.runInNewContext(
    ts.transpileModule(fs.readFileSync("src/shared/live.ts", "utf8"), {
      compilerOptions: {
        module: ts.ModuleKind.CommonJS,
        target: ts.ScriptTarget.ES2022,
      },
    }).outputText,
    {
      exports: m.exports,
      document,
      window,
      navigator,
      Date: { now: () => now },
      setTimeout: (fn, delay) => {
        timers.set(++id, { fn, at: now + delay });
        return id;
      },
      clearTimeout: (i) => timers.delete(i),
    },
  );
  const flush = async () => {
    for (let i = 0; i < 10; i++) await Promise.resolve();
  };
  async function advance(ms) {
    const end = now + ms;
    await flush();
    while (true) {
      const next = [...timers].sort((a, b) => a[1].at - b[1].at)[0];
      if (!next || next[1].at > end) break;
      now = next[1].at;
      timers.delete(next[0]);
      next[1].fn();
      await flush();
    }
    now = end;
    await flush();
  }
  return { ...m.exports, document, window, navigator, timers, advance, flush };
}
test("slow responses and repeated focus events never overlap; cleanup prevents future requests", async () => {
  const f = fixture(),
    c = new AbortController();
  let calls = 0,
    release;
  f.poll(
    () => {
      calls++;
      return new Promise((r) => (release = r));
    },
    { signal: c.signal, interval: 4000 },
  );
  await f.advance(60000);
  f.window.dispatchEvent(new Event("focus"));
  await f.advance(1000);
  assert.equal(calls, 1);
  release();
  await f.advance(3999);
  assert.equal(calls, 1);
  await f.advance(1);
  assert.equal(calls, 2);
  c.abort();
  release();
  await f.advance(60000);
  f.window.dispatchEvent(new Event("focus"));
  await f.advance(10000);
  assert.equal(calls, 2);
  assert.equal(f.timers.size, 0);
});
test("hidden views pause, visibility and network recovery wake them without a refresh button", async () => {
  const f = fixture(),
    c = new AbortController();
  let calls = 0;
  f.poll(
    async () => {
      calls++;
    },
    { signal: c.signal, interval: 4000 },
  );
  await f.flush();
  f.document.visibilityState = "hidden";
  f.document.dispatchEvent(new Event("visibilitychange"));
  await f.advance(60000);
  assert.equal(calls, 1);
  f.document.visibilityState = "visible";
  f.document.dispatchEvent(new Event("visibilitychange"));
  f.window.dispatchEvent(new Event("focus"));
  await f.advance(0);
  assert.equal(calls, 2);
  f.navigator.onLine = false;
  f.window.dispatchEvent(new Event("offline"));
  await f.advance(60000);
  assert.equal(calls, 2);
  f.navigator.onLine = true;
  f.window.dispatchEvent(new Event("online"));
  await f.advance(0);
  assert.equal(calls, 3);
  c.abort();
});
test("notification state keeps polling in background; failures back off and recover", async () => {
  const f = fixture(),
    c = new AbortController();
  let calls = 0;
  f.document.visibilityState = "hidden";
  f.poll(
    async () => {
      if (++calls < 3) throw Error("offline");
    },
    { signal: c.signal, interval: 1000, background: true },
  );
  await f.advance(1999);
  assert.equal(calls, 1);
  await f.advance(1);
  assert.equal(calls, 2);
  await f.advance(4000);
  assert.equal(calls, 3);
  await f.advance(1000);
  assert.equal(calls, 4);
  c.abort();
});
