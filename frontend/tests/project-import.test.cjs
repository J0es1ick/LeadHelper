const { test } = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const vm = require("node:vm");
const ts = require("typescript");
const { pathToFileURL } = require("node:url");
const path = require("node:path");

test("hidden Excel rows are excluded by default and may be deliberately included", async () => {
  const m = await model();
  const book = {
    sheets: [
      {
        name: "Projects",
        rows: [
          ["Name", "Status"],
          ["Visible", "Open"],
          ["Old", "Open"],
        ],
        hiddenRows: [2],
      },
    ],
  };
  const c = m.guessConfig(book.sheets[0], book);
  const a = m.prepareImport(book, [c], "x.xlsx");
  assert.equal(a.total, 1);
  assert.equal(a.skipped, 1);
  assert.equal(a.batches[0].projects[0].name, "Visible");
  c.includeHiddenRows = true;
  assert.equal(m.prepareImport(book, [c], "x.xlsx").total, 2);
});
async function model() {
  const ssf = await import(
    pathToFileURL(path.resolve("src/vendor/ssf/ssf.mjs"))
  );
  const m = { exports: {} };
  vm.runInNewContext(
    ts.transpileModule(
      fs.readFileSync("src/features/projects/project-import-model.ts", "utf8"),
      {
        compilerOptions: {
          module: ts.ModuleKind.CommonJS,
          target: ts.ScriptTarget.ES2022,
        },
      },
    ).outputText,
    { exports: m.exports, require: () => ssf, TextEncoder },
  );
  return m.exports;
}
test("all visible worksheets become independent folders with their own mapping", async () => {
  const m = await model();
  const book = {
    sheets: [
      {
        name: "Пресейл",
        rows: [
          ["Название", "Статус", "Ответственный"],
          ["Общий проект", "Оценка", "Анна"],
        ],
      },
      {
        name: "Delivery",
        rows: [
          ["Status", "Customer"],
          ["Done", "Общий проект"],
        ],
      },
      { name: "Hidden", hidden: true, rows: [["Name"], ["Draft"]] },
    ],
  };
  const configs = book.sheets.map((s) => m.guessConfig(s, book));
  assert.equal(configs[2].enabled, false);
  const result = m.prepareImport(book, configs, "team.xlsx");
  assert.equal(result.total, 2);
  assert.equal(result.batches.length, 2);
  assert.equal(result.batches[0].projects[0].group, "Пресейл");
  assert.equal(result.batches[1].projects[0].name, "Общий проект");
  assert.equal(result.batches[1].projects[0].state, "completed");
  assert.ok(result.batches[0].fields.includes("owner"));
  assert.ok(!result.batches[1].fields.includes("owner"));
  configs[1].folder = "Implementation";
  configs[1].completed = "Delivered";
  assert.equal(
    m.prepareImport(book, configs, "team.xlsx").batches[1].projects[0].state,
    "active",
  );
  assert.equal(
    m.prepareImport(book, configs, "team.xlsx").batches[1].projects[0].group,
    "Implementation",
  );
});
test("dated columns keep history and use last nonblank status, including Excel dates", async () => {
  const m = await model();
  const sheet = {
    name: "Matrix",
    rows: [
      ["Клиент", "46200", "46201", "46202"],
      ["Проект", "Оценка", "", "Разработка"],
    ],
    cells: {
      "0:1": { style: 1, number: 46200 },
      "0:2": { style: 1, number: 46201 },
      "0:3": { style: 1, number: 46202 },
    },
  };
  const book = { sheets: [sheet], styles: [{}, { format: "dd.mm.yyyy" }] };
  const config = m.guessConfig(sheet, book);
  assert.equal(config.mode, "history");
  const p = m.prepareImport(book, [config], "matrix.xlsx").batches[0]
    .projects[0];
  assert.equal(p.status, "Разработка");
  assert.equal(p.history.length, 2);
  assert.match(p.history[0].label, /\d\d\.\d\d\.2026/);
});
test("single-project history and excluded service rows are explicit", async () => {
  const m = await model();
  const book = {
    sheets: [
      {
        name: "Rollout",
        rows: [
          ["Дата", "Статус"],
          ["01.11.2026", "Подготовка"],
          ["02.11.2026", "Запуск"],
        ],
      },
    ],
  };
  const c = m.guessConfig(book.sheets[0], book);
  assert.equal(c.mode, "single");
  const p = m.prepareImport(book, [c], "work.xlsx").batches[0].projects[0];
  assert.equal(p.name, "Rollout");
  assert.equal(p.status, "Запуск");
  assert.equal(p.history.length, 2);
  const b = {
    sheets: [
      {
        name: "Rows",
        rows: [
          ["Name", "Status"],
          ["A", "Open"],
          ["", "Totals"],
        ],
      },
    ],
  };
  const cfg = m.guessConfig(b.sheets[0], b);
  assert.throws(() => m.prepareImport(b, [cfg], "x.xlsx"), /строке 3/);
  cfg.skipUnnamed = true;
  assert.equal(m.prepareImport(b, [cfg], "x.xlsx").skipped, 1);
  cfg.enabled = false;
  assert.throws(() => m.prepareImport(b, [cfg], "x.xlsx"), /хотя бы один/);
});
