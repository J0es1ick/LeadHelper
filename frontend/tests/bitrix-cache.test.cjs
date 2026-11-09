const { test } = require("node:test");
const assert = require("node:assert/strict");
const ts = require("typescript");
const fs = require("node:fs");
const vm = require("node:vm");
const m = { exports: {} };
let now = 0;
const clock = { now: () => now };
vm.runInNewContext(
  ts.transpileModule(
    fs.readFileSync("src/features/integrations/bitrix-cache.ts", "utf8"),
    {
      compilerOptions: {
        module: ts.ModuleKind.CommonJS,
        target: ts.ScriptTarget.ES2022,
      },
    },
  ).outputText,
  { exports: m.exports, Date: clock, require: () => ({}) },
);
const { DialogCache } = m.exports;
test("dialog cache isolates accounts and pages, expires and evicts least-recently-used data", () => {
  now = 0;
  const c = new DialogCache(2, 20000, 100);
  const value = (id) => ({ accountId: "a", items: [{ id }] });
  c.put("a", "1", "", value("one"));
  c.put("a", "2", "", value("two"));
  assert.equal(c.get("b", "1", ""), undefined);
  assert.equal(c.get("a", "1", "older"), undefined);
  assert.equal(c.get("a", "1", "").items[0].id, "one");
  c.put("a", "3", "", value("three"));
  assert.equal(c.get("a", "2", ""), undefined);
  c.put("a", "1", "", { accountId: "b", items: [] });
  assert.equal(c.get("a", "1", "").items[0].id, "one");
  now = 101;
  assert.equal(c.get("a", "1", ""), undefined);
});
test("oversized conversations cannot grow cache indefinitely", () => {
  now = 0;
  const c = new DialogCache(2, 100, 100);
  c.put("a", "1", "", { accountId: "a", items: [{ text: "x".repeat(1000) }] });
  assert.equal(c.get("a", "1", ""), undefined);
});
