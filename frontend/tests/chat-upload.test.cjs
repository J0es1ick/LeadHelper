const { test } = require("node:test"),
  assert = require("node:assert/strict"),
  ts = require("typescript"),
  fs = require("node:fs"),
  vm = require("node:vm");
let next = 0;
const m = { exports: {} };
vm.runInNewContext(
  ts.transpileModule(
    fs.readFileSync("src/features/integrations/chat-upload.ts", "utf8"),
    {
      compilerOptions: {
        module: ts.ModuleKind.CommonJS,
        target: ts.ScriptTarget.ES2022,
      },
    },
  ).outputText,
  {
    exports: m.exports,
    require: () => ({}),
    crypto: { randomUUID: () => `file-${++next}` },
  },
);
test("chat file selection rejects empty/oversized files, avoids duplicates and bounds the queue", () => {
  const { addChatFiles, chatFileLimit } = m.exports,
    first = { name: "a.txt", size: 10, lastModified: 1 };
  let result = addChatFiles(
    [],
    [
      first,
      first,
      { name: "empty", size: 0 },
      { name: "large", size: chatFileLimit + 1 },
    ],
  );
  assert.equal(result.items.length, 1);
  assert.match(result.error, /пустой/);
  assert.match(result.error, /25 МБ/);
  result = addChatFiles(
    result.items,
    Array.from({ length: 11 }, (_, i) => ({
      name: `${i}.txt`,
      size: 10,
      lastModified: 1,
    })),
  );
  assert.equal(result.items.length, 10);
  assert.match(result.error, /10 файлов/);
  result = addChatFiles(
    [],
    Array.from({ length: 5 }, (_, i) => ({
      name: `${i}.bin`,
      size: chatFileLimit,
      lastModified: 1,
    })),
  );
  assert.equal(result.items.length, 4);
  assert.match(result.error, /100 МБ/);
});
