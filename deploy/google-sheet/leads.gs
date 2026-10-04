// The receiving side of the lead hook, for a Google Sheet: a script bound to the sheet and deployed as a web app.
// The front door posts a lead as JSON when a demo starts and again when it ends, with the secret in the body, since a
// web app cannot read request headers. A lead is one row, found by its slug, added when it starts and filled in when it
// ends, so a lead that is sent twice is still one row. SECRET is a script property: run setup once to make it, and read
// it in Project Settings, under Script properties.

const COLUMNS = ['slug', 'requested', 'name', 'email', 'company', 'ready', 'ended', 'extended', 'outcome'];
const SHEET = 'Leads';

// Makes the secret, once. It is a script property, which is kept in Google and read by nothing but this script.
function setup() {
  const properties = PropertiesService.getScriptProperties();
  if (!properties.getProperty('SECRET')) {
    properties.setProperty('SECRET', Utilities.getUuid().replace(/-/g, '') + Utilities.getUuid().replace(/-/g, ''));
  }
  leadsSheet();
}

function doPost(e) {
  let body;
  try {
    body = JSON.parse(e.postData.contents);
  } catch (err) {
    return reply({ ok: false });
  }
  const secret = PropertiesService.getScriptProperties().getProperty('SECRET');
  const lead = body.lead || {};
  if (!secret || body.secret !== secret || !lead.slug) {
    return reply({ ok: false });
  }
  const lock = LockService.getScriptLock();
  lock.waitLock(20000);
  try {
    const sheet = leadsSheet();
    const values = [
      lead.slug, lead.requested, lead.name, lead.email, lead.company, lead.ready, lead.ended,
      lead.extended ? 'yes' : '', body.event === 'started' ? 'in progress' : lead.reason,
    ].map(text);
    const row = rowOf(sheet, lead.slug);
    if (row === 0) {
      sheet.appendRow(values);
    } else {
      sheet.getRange(row, 1, 1, values.length).setValues([values]);
    }
  } finally {
    lock.releaseLock();
  }
  return reply({ ok: true });
}

// A page opened in a browser says nothing about the sheet.
function doGet() {
  return reply({ ok: false });
}

function leadsSheet() {
  const book = SpreadsheetApp.getActiveSpreadsheet();
  let sheet = book.getSheetByName(SHEET);
  if (!sheet) {
    sheet = book.insertSheet(SHEET);
    sheet.appendRow(COLUMNS);
    sheet.setFrozenRows(1);
  }
  return sheet;
}

// The row of a lead, or 0 when there is none.
function rowOf(sheet, slug) {
  const last = sheet.getLastRow();
  if (last < 2) {
    return 0;
  }
  const slugs = sheet.getRange(2, 1, last - 1, 1).getValues();
  for (let i = 0; i < slugs.length; i++) {
    if (slugs[i][0] === slug) {
      return i + 2;
    }
  }
  return 0;
}

// What a visitor typed is kept as text, since a cell that begins with one of these characters is a formula to a sheet.
function text(value) {
  const s = value === undefined || value === null ? '' : String(value);
  return /^[=+\-@\t\r]/.test(s) ? "'" + s : s;
}

function reply(answer) {
  return ContentService.createTextOutput(JSON.stringify(answer)).setMimeType(ContentService.MimeType.JSON);
}
