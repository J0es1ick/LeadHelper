# PDF.js 5.6.205

Unmodified Mozilla PDF.js browser distribution, copied from the installed primary
runtime's `pdfjs-dist` package. Licensed under Apache-2.0; see LICENSE. Source:
https://github.com/mozilla/pdf.js — https://mozilla.github.io/pdf.js/

`pdf.mjs` is the upstream `build/pdf.min.mjs`; the worker is
`build/pdf.worker.min.mjs`. CMaps, standard fonts, ICC profiles and WASM decoders
are in `frontend/public/vendor/pdfjs`, with their upstream license files.
The adjacent type declarations are the small interface used by LeadHelper.

The dependency is vendored because this environment cannot reach npm. Update the
library, worker and rendering assets together. PDF rendering is lazy-loaded,
runs locally, and does not execute document JavaScript or load remote viewers.
