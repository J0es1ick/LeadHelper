const { test } = require("node:test"),
  assert = require("node:assert/strict"),
  fs = require("node:fs"),
  vm = require("node:vm"),
  crypto = require("node:crypto");
function fixture() {
  const batches = [];
  const s = {
    getName: () => "Projects",
    getSheetId: () => 1,
    getLastRow: () => 2,
    getMaxRows: () => 100,
    getMaxColumns: () => 30,
    getRange: () => ({ getFormula: () => "" }),
  };
  const metaSheet = {
    getSheetId: () => 99,
    getMaxRows: () => 100,
    getMaxColumns: () => 3,
  };
  const ctx = vm.createContext({
    Utilities: {
      DigestAlgorithm: { SHA_256: "sha256" },
      computeDigest: (_, text) =>
        Array.from(crypto.createHash("sha256").update(text).digest()),
      formatDate: () => "09.11.2026",
    },
    Sheets: { Spreadsheets: { batchUpdate: (body) => batches.push(body) } },
    ContentService: {
      MimeType: { JSON: "json" },
      createTextOutput: (text) => ({ setMimeType: () => JSON.parse(text) }),
    },
    PropertiesService: {
      getScriptProperties: () => ({
        getProperty: (key) => (key === "SECRET" ? "correct-secret" : "book"),
      }),
    },
  });
  vm.runInContext(fs.readFileSync("public/project-sheets.gs", "utf8"), ctx);
  const data = {
    id: 1,
    rows: [
      ["Клиент", "Статус 08.11.2026", "Ответственный"],
      ["A", "Old", "Ann"],
    ],
    rowIds: { 1: "stable" },
    revisions: { 1: "r1" },
  };
  ctx.lhSnapshot = () => ({ sheets: [data] });
  ctx.lhMeta = () => ({ sheet: metaSheet, entries: {}, nextRow: 2 });
  const book = {
    getId: () => "book",
    getSheets: () => [s],
    getSpreadsheetTimeZone: () => "Europe/Moscow",
  };
  const project = {
    id: "p",
    name: "A",
    status: "New",
    owner: "Bob",
    description: "",
    state: "active",
    startDate: "",
    dueDate: "",
    url: "",
    sheet: {
      spreadsheetId: "book",
      sheetId: 1,
      rowId: "stable",
      mode: "matrix",
    },
  };
  const base = {
    ...project,
    status: "Old",
    owner: "Ann",
    sheet: { revision: "r1" },
  };
  return { ctx, batches, book, project, base, data };
}

test("visibility reads bounded row metadata once and handles filtered rows and offsets", () => {
  let calls = 0;
  const ctx = vm.createContext({
    Sheets: {
      Spreadsheets: {
        get: (id, options) => {
          calls++;
          assert.equal(id, "book");
          assert.deepEqual(Array.from(options.ranges), [
            "'Team''s projects'!A1:A4",
          ]);
          assert.match(options.fields, /hiddenByUser,hiddenByFilter/);
          return {
            sheets: [
              {
                properties: { sheetId: 1 },
                data: [
                  {
                    startRow: 1,
                    rowMetadata: [
                      { hiddenByUser: true },
                      { hiddenByFilter: true },
                      {},
                    ],
                  },
                ],
              },
            ],
          };
        },
      },
    },
  });
  vm.runInContext(fs.readFileSync("public/project-sheets.gs", "utf8"), ctx);
  const sheet = {
    getName: () => "Team's projects",
    getLastRow: () => 4,
    getLastColumn: () => 3,
    getSheetId: () => 1,
  };
  const result = ctx.lhRowVisibility({ getId: () => "book" }, [sheet]);
  assert.equal(calls, 1);
  assert.deepEqual(Array.from(result[1]), [1, 2]);
  ctx.Sheets.Spreadsheets.get = () => ({ sheets: [] });
  assert.throws(
    () => ctx.lhRowVisibility({ getId: () => "book" }, [sheet]),
    /видимость строк/,
  );
});
test("sheet update is atomic, guarded by source revision and literal strings", () => {
  const { ctx, batches, book, project, base } = fixture();
  ctx.lhApply(book, { project, base, operationId: "p:2" });
  assert.equal(batches.length, 1);
  const r = batches[0].requests;
  assert.equal(r.filter((x) => x.updateCells).length, 6);
  assert.ok(
    r.some(
      (x) =>
        x.updateCells?.rows[0].values[0].userEnteredValue.stringValue === "New",
    ),
  );
  assert.ok(
    r.some(
      (x) =>
        x.updateCells?.range.startColumnIndex === 2 &&
        x.updateCells.rows[0].values[0].userEnteredValue.stringValue === "Bob",
    ),
  );
  assert.ok(
    !r.some(
      (x) => x.updateCells?.rows[0].values[0].userEnteredValue.formulaValue,
    ),
  );
});
test("stale source cannot be overwritten", () => {
  const { ctx, batches, book, project, base } = fixture();
  base.sheet.revision = "stale";
  assert.throws(
    () => ctx.lhApply(book, { project, base, operationId: "p:2" }),
    /изменилась/,
  );
  assert.equal(batches.length, 0);
});
test("retry with saved receipt does not duplicate history", () => {
  const { ctx, batches, book, project, base } = fixture();
  ctx.lhMeta = () => ({ entries: { "1:stable": { operation: "p:2" } } });
  ctx.lhApply(book, { project, base, operationId: "p:2" });
  assert.equal(batches.length, 0);
});
test("revision ignores empty trailing columns but includes meaningful source changes", () => {
  const { ctx } = fixture();
  const s = {
    mode: "matrix",
    rows: [
      ["Name", "Status"],
      ["A", "Old"],
    ],
  };
  const a = ctx.lhRevision(s, 1, {});
  s.rows[0].push("New column");
  s.rows[1].push("");
  assert.equal(ctx.lhRevision(s, 1, {}), a);
  s.rows[1][1] = "New";
  assert.notEqual(ctx.lhRevision(s, 1, {}), a);
});
test("unauthenticated bridge requests never access a spreadsheet", () => {
  const { ctx } = fixture();
  assert.equal(
    ctx.doPost({
      postData: {
        contents: JSON.stringify({ token: "wrong", action: "snapshot" }),
      },
    }).ok,
    false,
  );
});
