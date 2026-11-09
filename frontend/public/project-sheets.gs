/* LeadHelper: bound Apps Script. Enable the Google Sheets advanced service.
 * Run setupLeadHelper once, then deploy as a web app executing as yourself.
 * Access: Anyone. Every request is authenticated by a 64-character secret.
 * The spreadsheet itself does not need public access.
 */
const LH_META = 'leadhelper.project.row';
const LH_DATA = '__LeadHelper';

function setupLeadHelper() {
  const book = SpreadsheetApp.getActiveSpreadsheet();
  const properties = PropertiesService.getScriptProperties();
  properties.setProperty('BOOK_ID', book.getId());
  if (!properties.getProperty('SECRET')) properties.setProperty('SECRET', Utilities.getUuid().replace(/-/g, '') + Utilities.getUuid().replace(/-/g, ''));
  if (!book.getSheetByName(LH_DATA)) {
    const sheet = book.insertSheet(LH_DATA);
    sheet.getRange(1, 1, 1, 3).setValues([['Project key', 'Fields', 'Operation']]);
    sheet.hideSheet();
  }
  lhSnapshot(book, true);
  SpreadsheetApp.getUi().alert('Секрет LeadHelper', properties.getProperty('SECRET'), SpreadsheetApp.getUi().ButtonSet.OK);
}

function doPost(event) {
  let lock;
  try {
    const body = JSON.parse(event.postData.contents);
    const properties = PropertiesService.getScriptProperties();
    const secret = properties.getProperty('SECRET');
    if (!secret || body.token !== secret) return lhResult({ ok: false, error: 'Неверный секрет подключения' });
    lock = LockService.getScriptLock();
    if (!lock.tryLock(5000)) return lhResult({ ok: false, error: 'Синхронизация занята, повторите позднее' });
    const book = SpreadsheetApp.openById(properties.getProperty('BOOK_ID'));
    if (body.action === 'snapshot') return lhResult({ ok: true, snapshot: lhSnapshot(book, true) });
    if (body.action === 'apply') { lhApply(book, body.change); return lhResult({ ok: true }); }
    return lhResult({ ok: false, error: 'Неизвестное действие' });
  } catch (error) {
    // Never return OAuth tokens, request bodies, or a stack trace.
    const message = String(error.message || 'Ошибка');
    return lhResult({ ok: false, error: message.startsWith('LH:') ? message.slice(3) : 'Не удалось обработать таблицу. Проверьте доступ и сервис Google Sheets API в Apps Script.' });
  } finally { if (lock && lock.hasLock()) lock.releaseLock(); }
}
function lhResult(value) { return ContentService.createTextOutput(JSON.stringify(value)).setMimeType(ContentService.MimeType.JSON); }
function lhKey(sheet, rowId) { return sheet + ':' + rowId; }
function lhMeta(book) {
  const sheet = book.getSheetByName(LH_DATA);
  if (!sheet) throw Error('LH:Сначала выполните setupLeadHelper');
  const values = sheet.getDataRange().getDisplayValues();
  const result = {};
  values.slice(1).forEach((r, i) => { if (r[0]) result[r[0]] = { row: i + 2, fields: JSON.parse(r[1] || '{}'), operation: r[2] || '' }; });
  return { sheet, entries: result, nextRow: values.length + 1 };
}
function lhTrim(row) { const copy = row.slice(); while (copy.length && copy[copy.length - 1] === '') copy.pop(); return copy; }
function lhRevision(sheet, row, fields) {
  const value = sheet.mode === 'vertical' ? sheet.rows : [sheet.rows[0].slice(0, lhTrim(sheet.rows[row] || []).length), lhTrim(sheet.rows[row] || [])];
  const raw = JSON.stringify([value, fields]);
  return Utilities.computeDigest(Utilities.DigestAlgorithm.SHA_256, raw).map(b => ('0' + ((b + 256) % 256).toString(16)).slice(-2)).join('');
}
function lhRowVisibility(book, sheets) {
  const ranges = sheets.filter(s => s.getLastRow() && s.getLastColumn()).map(s => {
    if (s.getLastRow() > 5000) throw Error('LH:В листе больше 5000 строк');
    return "'" + s.getName().replace(/'/g, "''") + "'!A1:A" + s.getLastRow();
  });
  if (!ranges.length) return {};
  const response = Sheets.Spreadsheets.get(book.getId(), {
    ranges, fields: 'sheets(properties(sheetId),data(startRow,rowMetadata(hiddenByUser,hiddenByFilter)))'
  });
  const result = {};
  for (const sheet of response.sheets || []) {
    const hidden = [];
    for (const block of sheet.data || []) (block.rowMetadata || []).forEach((row, i) => {
      if (row.hiddenByUser || row.hiddenByFilter) hidden.push((block.startRow || 0) + i);
    });
    result[sheet.properties.sheetId] = hidden;
  }
  for (const sheet of sheets) if (sheet.getLastRow() && sheet.getLastColumn() && !Object.prototype.hasOwnProperty.call(result, sheet.getSheetId())) throw Error('LH:Не удалось прочитать видимость строк таблицы');
  return result;
}
function lhSnapshot(book, assign) {
  const sourceSheets = book.getSheets().filter(s => s.getName() !== LH_DATA);
  if (sourceSheets.length > 40) throw Error('LH:В книге больше 40 листов');
  const hiddenRows = lhRowVisibility(book, sourceSheets);
  const meta = lhMeta(book), result = { visibilityKnown: true, spreadsheetId: book.getId(), title: book.getName(), fetchedAt: new Date().toISOString(), sheets: [] };
  let cells = 0;
  for (const sheet of sourceSheets) {
    const height = sheet.getLastRow(), width = sheet.getLastColumn();
    if (!height || !width) continue;
    cells += height * width;
    if (height > 5000 || width > 400 || cells > 300000 || result.sheets.length >= 40) throw Error('LH:Таблица превышает лимит 300 000 ячеек, 5000 строк или 400 столбцов');
    const range = sheet.getRange(1, 1, height, width), rows = range.getDisplayValues(), notes = range.getNotes(), rich = range.getRichTextValues();
    const data = { id: sheet.getSheetId(), name: sheet.getName(), rows, hiddenRows: hiddenRows[sheet.getSheetId()] || [], boldRows: [], rowIds: {}, notes: {}, links: {}, hidden: sheet.isSheetHidden(), revisions: {}, overrides: {} };
    const vertical = rows[0][0].trim().toLowerCase() === 'дата' && (rows[0][1] || '').trim().toLowerCase() === 'статус';
    const single = /\d{1,2}[./]\d{1,2}|статус|статутс/i.test(rows[0][0]);
    data.mode = vertical ? 'vertical' : single ? 'single' : 'matrix';
    const weights = sheet.getRange(1, 1, height, 1).getFontWeights();
    weights.forEach((w, i) => { if (w[0] === 'bold') data.boldRows.push(i); });
    const existing = sheet.createDeveloperMetadataFinder().withKey(LH_META).find();
    for (const m of existing) {
      const row = m.getLocation().getRow();
      if (row) { const n = row.getRow() - 1; if (data.rowIds[n]) throw Error('LH:Повторяющиеся метки строк. Удалите дубликат служебной метки'); data.rowIds[n] = m.getValue(); }
    }
    const requests = [];
    for (let r = 0; r < height; r++) {
      for (let c = 0; c < width; c++) { if (notes[r][c]) data.notes[r + ':' + c] = notes[r][c]; const link = rich[r][c] && rich[r][c].getLinkUrl(); if (link) data.links[r + ':' + c] = link; }
      if (data.mode === 'matrix' && r > 0 && rows[r][0].trim() && !data.rowIds[r]) {
        if (!assign) continue;
        const id = Utilities.getUuid(); data.rowIds[r] = id;
        requests.push({ createDeveloperMetadata: { developerMetadata: { metadataKey: LH_META, metadataValue: id, visibility: 'DOCUMENT', location: { dimensionRange: { sheetId: data.id, dimension: 'ROWS', startIndex: r, endIndex: r + 1 } } } } });
      }
      if (data.mode === 'vertical' && r !== 0 || data.mode === 'single' && r !== 1 || data.mode === 'matrix' && !data.rowIds[r]) continue;
      const id = data.mode === 'matrix' ? data.rowIds[r] : 'sheet';
      const extra = meta.entries[lhKey(data.id, id)];
      data.overrides[r] = extra ? extra.fields : {};
      data.revisions[r] = lhRevision(data, r, data.overrides[r]);
    }
    if (requests.length) Sheets.Spreadsheets.batchUpdate({ requests }, book.getId());
    delete data.mode;
    result.sheets.push(data);
  }
  return result;
}
function lhCell(sheetId, row, col, value) {
  // stringValue never evaluates user text as a formula.
  return { updateCells: { range: { sheetId, startRowIndex: row, endRowIndex: row + 1, startColumnIndex: col, endColumnIndex: col + 1 }, rows: [{ values: [{ userEnteredValue: { stringValue: String(value) } }] }], fields: 'userEnteredValue' } };
}
function lhCapacity(requests, sheet, rows, cols) {
  if (rows > sheet.getMaxRows()) requests.push({ appendDimension: { sheetId: sheet.getSheetId(), dimension: 'ROWS', length: rows - sheet.getMaxRows() } });
  if (cols > sheet.getMaxColumns()) requests.push({ appendDimension: { sheetId: sheet.getSheetId(), dimension: 'COLUMNS', length: cols - sheet.getMaxColumns() } });
}
function lhApply(book, change) {
  if (!change || !change.project || !change.project.sheet || !/^[a-zA-Z0-9:-]{1,150}$/.test(change.operationId)) throw Error('LH:Некорректное изменение');
  const p = change.project, base = change.base || {}, ref = p.sheet;
  if (ref.spreadsheetId !== book.getId()) throw Error('LH:Изменение относится к другой книге');
  for (const field of ['name', 'status', 'owner', 'description', 'state', 'startDate', 'dueDate', 'url']) if (typeof p[field] !== 'string' || p[field].length > 45000) throw Error('LH:Некорректное поле проекта');
  if (!p.name.trim() || !['active', 'completed', 'archived'].includes(p.state)) throw Error('LH:Проверьте название и раздел проекта');
  const meta = lhMeta(book), requests = [];
  let sheet, row, key, data, sheetId;
  if (ref.mode === 'new') {
    // Idempotency is checked before any structural or content mutation.
    const matches = book.createDeveloperMetadataFinder().withKey(LH_META).withValue(ref.rowId).find();
    if (matches.length) return;
    sheet = book.getSheetByName('Проекты LeadHelper');
    if (sheet) { sheetId = sheet.getSheetId(); row = sheet.getLastRow(); }
    else { sheetId = 1 + Math.floor(Math.random() * 1000000000); row = 1; requests.push({ addSheet: { properties: { sheetId, title: 'Проекты LeadHelper', gridProperties: { rowCount: 1000, columnCount: 26, frozenRowCount: 1 } } } }); requests.push(lhCell(sheetId, 0, 0, 'Проект')); }
    if (row > 4999) throw Error('LH:Достигнут предел 5000 строк');
    if (sheet) lhCapacity(requests, sheet, row + 1, 2);
    requests.push(lhCell(sheetId, row, 0, p.name));
    requests.push({ createDeveloperMetadata: { developerMetadata: { metadataKey: LH_META, metadataValue: ref.rowId, visibility: 'DOCUMENT', location: { dimensionRange: { sheetId, dimension: 'ROWS', startIndex: row, endIndex: row + 1 } } } } });
    data = { rows: sheet ? sheet.getDataRange().getDisplayValues() : [['Проект']] };
    key = lhKey(sheetId, ref.rowId);
  } else {
    sheet = book.getSheets().find(s => s.getSheetId() === ref.sheetId);
    if (!sheet || sheet.getName() === LH_DATA) throw Error('LH:Лист не найден');
    sheetId = sheet.getSheetId();
    key = lhKey(sheetId, ref.rowId);
    if (meta.entries[key] && meta.entries[key].operation === change.operationId) return;
    const snapshot = lhSnapshot(book, false);
    data = snapshot.sheets.find(s => s.id === sheetId);
    row = ref.mode === 'vertical' ? 0 : ref.mode === 'single' ? 1 : Number(Object.keys(data.rowIds).find(r => data.rowIds[r] === ref.rowId));
    if (!Number.isInteger(row) || !data.rows[row]) throw Error('LH:Строка была удалена. Обновите проекты');
    if (!base.sheet || !base.sheet.revision || data.revisions[row] !== base.sheet.revision) throw Error('LH:Таблица изменилась во время синхронизации. Повторная проверка будет выполнена автоматически');
    if (p.name !== base.name && ref.mode === 'matrix') {
      if (sheet.getRange(row + 1, 1).getFormula()) throw Error('LH:Название рассчитано формулой; измените его в таблице');
      requests.push(lhCell(sheetId, row, 0, p.name));
    }
  }
  const date = Utilities.formatDate(new Date(), book.getSpreadsheetTimeZone(), 'dd.MM.yyyy');
  if (p.status !== base.status) {
    if (ref.mode === 'vertical') {
      const next = sheet.getLastRow(); if (next >= 5000) throw Error('LH:Достигнут предел 5000 строк');
      lhCapacity(requests, sheet, next + 1, 2);
      requests.push(lhCell(sheetId, next, 0, date), lhCell(sheetId, next, 1, p.status));
    } else {
      const header = 'Статус ' + date + ' (LeadHelper)';
      let col = data.rows[0].indexOf(header);
      if (col < 0) col = lhTrim(data.rows[0]).length;
      if (col >= 400) throw Error('LH:Достигнут предел 400 столбцов');
      if (sheet) { lhCapacity(requests, sheet, row + 1, col + 1); const target = sheet.getRange(Math.min(row + 1, sheet.getMaxRows()), Math.min(col + 1, sheet.getMaxColumns())); if (col < sheet.getMaxColumns() && row < sheet.getMaxRows() && target.getFormula()) throw Error('LH:Статус рассчитан формулой; измените его в таблице'); }
      requests.push(lhCell(sheetId, 0, col, header), lhCell(sheetId, row, col, p.status));
    }
  }
  const fields = { ...((meta.entries[key] || {}).fields || {}) };
  for (const field of p.fields || []) {
    const previous = (base.fields || []).find(f => f.key === field.key);
    if (!previous || field.value === previous.value) continue;
    if (!/^c[1-9][0-9]*$/.test(field.key) || field.name !== previous.name) throw Error('LH:Столбец таблицы изменился. Обновите проект');
    const col = Number(field.key.slice(1)) - 1;
    if (!sheet || !data.rows[0][col] || data.rows[0][col].trim() !== field.name || ref.mode !== 'matrix') throw Error('LH:Поле доступно для изменения только в исходной таблице');
    if (sheet.getRange(row + 1, col + 1).getFormula()) throw Error('LH:Поле рассчитано формулой. Измените его в исходной таблице');
    requests.push(lhCell(sheetId, row, col, field.value));
  }
  // Matrix names stay in column A; other fields are retained in a private service tab.
  for (const field of ['owner', 'description', 'state', 'startDate', 'dueDate', 'url']) if (p[field] !== base[field]) fields[field] = p[field];
  if (p.owner !== base.owner && ref.mode === 'matrix') {
    const ownerColumn = data.rows[0].findIndex(value => /ответствен/i.test(value));
    if (ownerColumn >= 0) {
      if (sheet.getRange(row + 1, ownerColumn + 1).getFormula()) throw Error('LH:Ответственный рассчитан формулой; измените его в таблице');
      requests.push(lhCell(sheetId, row, ownerColumn, p.owner));
      delete fields.owner;
    }
  }
  if (ref.mode !== 'matrix' && ref.mode !== 'new' && p.name !== base.name) fields.name = p.name;
  const metaRow = meta.entries[key] ? meta.entries[key].row : meta.nextRow;
  lhCapacity(requests, meta.sheet, metaRow, 3);
  requests.push(lhCell(meta.sheet.getSheetId(), metaRow - 1, 0, key), lhCell(meta.sheet.getSheetId(), metaRow - 1, 1, JSON.stringify(fields)), lhCell(meta.sheet.getSheetId(), metaRow - 1, 2, change.operationId));
  // All project writes and the operation receipt succeed atomically.
  Sheets.Spreadsheets.batchUpdate({ requests }, book.getId());
}
