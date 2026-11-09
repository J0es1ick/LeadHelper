# Local document preview dependencies

- docx-preview, package version 0.4.1, Apache-2.0; source: https://github.com/VolodymyrBaydalka/docxjs ; distributed JS blob SHA `8a59b672f062f5b6471718cee721804f2ae8dc11`. License in docx-preview.LICENSE.
- JSZip 3.10.1, MIT; copied from installed runtime package, license in jszip.LICENSE. Source: https://github.com/Stuk/jszip .
- Numeric/date formatting: SheetJS SSF 0.11.2, Apache-2.0, vendored under frontend/src/vendor/ssf. Source: https://github.com/SheetJS/ssf ; source blob SHA `37eba7a0cbef5319e7bdb0300e8c8be33b72f09f`. Local changes: ESM default export for bundling; unescaped dots without fractional-second zeros are treated as literal date separators (e.g. dd.mm.yyyy).

The viewer uses only locally shipped scripts; document contents are not sent to an online viewer. The iframe is sandboxed without same-origin permission. External relationships and embedded active contents are disabled. This is a read-only HTML rendition, not Microsoft Office; pagination and advanced document features may differ.
