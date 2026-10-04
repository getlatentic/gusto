// Runs the script against stand-ins for Google's services: node deploy/google-sheet/leads.test.js
const fs = require("fs"), vm = require("vm"), assert = require("assert");
function world({ mailFails = false } = {}) {
  const rows = [["slug"]]; const mails = [];
  const sheet = {
    getLastRow: () => rows.length, appendRow: (r) => rows.push(r), setFrozenRows() {},
    getRange: (r, c, n, m) => ({ getValues: () => rows.slice(1).map((x) => [x[0]]), setValues: (v) => { rows[r - 1] = v[0]; } }),
  };
  const ctx = {
    PropertiesService: { getScriptProperties: () => ({ getProperty: (k) => ({ SECRET: "s3" }[k] || null) }) },
    LockService: { getScriptLock: () => ({ waitLock() {}, releaseLock() {} }) },
    SpreadsheetApp: { getActiveSpreadsheet: () => ({ getSheetByName: () => sheet }) },
    ContentService: { createTextOutput: (t) => ({ t, setMimeType() { return this; } }), MimeType: { JSON: 1 } },
    Session: { getEffectiveUser: () => ({ getEmail: () => "owner@example.com" }) },
    MailApp: { sendEmail: (m) => { if (mailFails) throw new Error("quota"); mails.push(m); } },
    console, Utilities: { getUuid: () => "x" },
  };
  vm.createContext(ctx); vm.runInContext(fs.readFileSync(__dirname + "/leads.gs", "utf8"), ctx);
  const post = (event, lead) => JSON.parse(ctx.doPost({ postData: { contents: JSON.stringify({ event, secret: "s3", lead }) } }).t);
  return { rows, mails, post };
}
const lead = { slug: "a", name: "Ada\nBcc: x@evil.test", email: "ada@acme.com", company: "Acme", requested: "t" };
let w = world(); assert.deepEqual(w.post("started", lead), { ok: true });
assert.equal(w.mails.length, 1); assert.equal(w.mails[0].to, "owner@example.com");
assert(!/[\r\n]/.test(w.mails[0].subject), "the subject holds a line break"); assert(w.mails[0].body.includes("ada@acme.com"));
w.post("started", lead); assert.equal(w.mails.length, 1, "a lead sent twice is mailed twice");
w.post("ended", { ...lead, ended: "e", reason: "time" }); assert.equal(w.mails.length, 1, "an end is mailed");
assert.equal(w.rows.length, 2);
w = world({ mailFails: true }); assert.deepEqual(w.post("started", lead), { ok: true }); assert.equal(w.rows.length, 2, "the row is lost with the mail");
console.log("ok");
