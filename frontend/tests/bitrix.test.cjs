const { test } = require("node:test");
const assert = require("node:assert/strict");
const ts = require("typescript");
const fs = require("node:fs");
const vm = require("node:vm");
const m = { exports: {} };
vm.runInNewContext(
  ts.transpileModule(
    fs.readFileSync("src/features/integrations/bitrix-model.ts", "utf8"),
    {
      compilerOptions: {
        module: ts.ModuleKind.CommonJS,
        target: ts.ScriptTarget.ES2022,
      },
    },
  ).outputText,
  { exports: m.exports, URL },
);
const { departmentTree, sortMessages, plainRemoteText } = m.exports;
test("company tree keeps leaders, nested teams and orphan departments", () => {
  const nodes = departmentTree([
    { id: "1", parent: "0", headName: "Анна" },
    { id: "2", parent: "1" },
    { id: "3", parent: "2" },
    { id: "4", parent: "missing" },
  ]);
  assert.equal(nodes.length, 2);
  assert.equal(nodes[0].item.headName, "Анна");
  assert.equal(nodes[0].children[0].children[0].item.id, "3");
  assert.equal(nodes[1].item.id, "4");
});
test("cycles and self-parent cannot hide departments or recurse forever", () => {
  const nodes = departmentTree([
    { id: "1", parent: "2" },
    { id: "2", parent: "1" },
    { id: "3", parent: "3" },
  ]);
  const ids = [];
  const walk = (items) =>
    items.forEach((n) => {
      ids.push(n.item.id);
      walk(n.children);
    });
  walk(nodes);
  assert.equal(ids.length, 3);
  assert.equal(new Set(ids).size, 3);
});
test("chat history shows oldest first with numeric ID tie break and own replies", () => {
  const source = [
    { id: "12", date: "2026-11-07T10:00:00Z", own: true },
    { id: "9", date: "2026-11-07T10:00:00Z" },
    { id: "20", date: "2026-11-06T10:00:00Z" },
  ];
  const sorted = sortMessages(source);
  assert.equal(sorted.map((m) => m.id).join(","), "20,9,12");
  assert.equal(sorted[2].own, true);
  assert.equal(source[0].id, "12");
});
test("provider markup becomes readable plain text without executing HTML", () => {
  assert.equal(
    plainRemoteText("[USER=7]Анна[/USER] [B]текст[/B]"),
    "Анна текст",
  );
  assert.equal(plainRemoteText("<script>alert(1)</script>"), "");
});

test("news typography does not leak Bitrix service markup", () => {
  assert.equal(
    plainRemoteText(
      "[CENTER][SIZE=16][FONT=Arial][COLOR=#333]Новость[/COLOR][/FONT][/SIZE][/CENTER]",
    ),
    "Новость",
  );
  assert.equal(
    plainRemoteText("[LIST][*]Первое[*]Второе[/LIST]"),
    "• Первое\n• Второе",
  );
  assert.match(plainRemoteText("[DISK FILE ID=123]"), /Вложение/);
});

test("system mentions with REPLACE, all-user mentions and bot actions keep names, not service attributes", () => {
  assert.equal(
    plainRemoteText(
      "[USER=726 REPLACE]Евгений Крылов[/USER] пригласил в чат [USER=5120 REPLACE]Дмитрий Нарзуллаев[/USER], [USER=7752 REPLACE]Игорь Павлов[/USER]",
    ),
    "Евгений Крылов пригласил в чат Дмитрий Нарзуллаев, Игорь Павлов",
  );
  assert.equal(
    plainRemoteText(
      "[user=all replace][B]Все[/B][/user] [CHAT=42 REPLACE]Команда[/CHAT] [CONTEXT=chat42/99]Ответ[/CONTEXT] [SEND=/help]Помощь[/SEND]",
    ),
    "Все Команда Ответ Помощь",
  );
  assert.equal(
    plainRemoteText("Анна пригласила [USER=777 REPLA"),
    "Анна пригласила",
  );
});
test("literal code, brackets and legacy HTML retain meaning without HTML injection", () => {
  assert.equal(
    plainRemoteText("[Важно] [CODE]const x = [USER=1]; <b>literal</b>[/CODE]"),
    "[Важно] const x = [USER=1]; <b>literal</b>",
  );
  assert.equal(
    plainRemoteText(
      "<p>Текст &amp; <strong>оформление</strong></p><script>bad()</script>",
    ),
    "Текст & оформление",
  );
  assert.equal(
    plainRemoteText(
      '[ICON=https://example.test/a.png title="Готово" size=20] [IMG SIZE=medium]https://example.test/a.jpg[/IMG]',
    ),
    "Готово Изображение",
  );
  for (const raw of [
    "javascript:alert(1)",
    "data:text/html,bad",
    "https://user:pass@example.test",
    "https://example.test/rest/7/secret/",
    "https://example.test/a?auth=secret",
  ])
    assert.equal(m.exports.safeTextURL(raw), "");
  assert.equal(
    m.exports.safeTextURL("https://example.test/plan"),
    "https://example.test/plan",
  );
  const tree = m.exports.parseRemoteText(
    "[URL=https://example.test/plan][B]План[/B][/URL]",
  );
  assert.equal(
    tree
      .find((n) => n.tag === "URL")
      .children.find((n) => n.tag === "B")
      .children.join(""),
    "План",
  );
});
