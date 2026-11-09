const test = require("node:test");
const assert = require("node:assert/strict");
test("Office number formats preserve dates, fractional seconds, numbers and the 1904 calendar", async () => {
  const { default: SSF } = await import("../src/vendor/ssf/ssf.mjs");
  assert.equal(SSF.format("dd.mm.yyyy", 46335), "09.11.2026");
  assert.equal(SSF.format("yyyy-mm-dd", 46335), "2026-11-09");
  assert.equal(
    SSF.format("dd.mm.yyyy", 44873, { date1904: true }),
    "09.11.2026",
  );
  assert.equal(SSF.format("hh:mm:ss.000", 0.5), "12:00:00.000");
  assert.equal(SSF.format("0.00", 8.5), "8.50");
  assert.equal(SSF.format("0%", 0.5), "50%");
});
