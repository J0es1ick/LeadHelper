const { test } = require("node:test"),
  assert = require("node:assert/strict"),
  fs = require("node:fs"),
  ts = require("typescript"),
  vm = require("node:vm");
const m = { exports: {} };
vm.runInNewContext(
  ts.transpileModule(
    fs.readFileSync("src/features/team/export-periods.ts", "utf8"),
    {
      compilerOptions: {
        module: ts.ModuleKind.CommonJS,
        target: ts.ScriptTarget.ES2022,
      },
    },
  ).outputText,
  { exports: m.exports },
);
const periods = (options) =>
  JSON.parse(JSON.stringify(m.exports.exportPeriods(options)));
const base = {
  from: "2026-11-09",
  to: "2026-11-15",
  split: "weekly",
  boundary: "start",
  weekdays: [1, 4],
  dates: [],
};
test("Monday/Thursday starts produce Mon-Wed and Thu-Sun, never cumulative ranges", () => {
  assert.deepEqual(periods(base), [
    { from: "2026-11-09", to: "2026-11-11" },
    { from: "2026-11-12", to: "2026-11-15" },
  ]);
  assert.deepEqual(periods({ ...base, boundary: "end" }), [
    { from: "2026-11-09", to: "2026-11-09" },
    { from: "2026-11-10", to: "2026-11-12" },
    { from: "2026-11-13", to: "2026-11-15" },
  ]);
});
test("one day, leap day, unsorted duplicate dates and inclusive last day", () => {
  assert.deepEqual(periods({ ...base, to: base.from, split: "none" }), [
    { from: base.from, to: base.from },
  ]);
  assert.deepEqual(
    periods({
      ...base,
      from: "2024-02-28",
      to: "2024-03-01",
      split: "dates",
      dates: ["2024-03-01", "2024-02-29", "2024-02-29"],
    }),
    [
      { from: "2024-02-28", to: "2024-02-28" },
      { from: "2024-02-29", to: "2024-02-29" },
      { from: "2024-03-01", to: "2024-03-01" },
    ],
  );
  for (const value of [
    { from: "2026-02-30" },
    { to: "2026-11-08" },
    { weekdays: [] },
    { split: "dates", dates: ["2026-11-16"] },
    { from: "2024-01-01", to: "2025-01-01" },
  ])
    assert.throws(() => periods({ ...base, ...value }));
});
