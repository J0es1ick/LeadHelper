// Minimal types for the PDF.js APIs used by LeadHelper. Runtime: 5.6.205.
export interface RenderTask { promise: Promise<void>; cancel(): void }
export interface PDFPageProxy {
  getViewport(options: { scale: number }): { width: number; height: number };
  render(options: { canvas: HTMLCanvasElement; viewport: { width: number; height: number } }): RenderTask;
}
export interface PDFDocumentProxy {
  numPages: number;
  getPage(page: number): Promise<PDFPageProxy>;
}
export interface PDFDocumentLoadingTask {
  promise: Promise<PDFDocumentProxy>;
  destroy(): Promise<void>;
}
export const GlobalWorkerOptions: { workerSrc: string };
export function getDocument(options: Record<string, unknown>): PDFDocumentLoadingTask;
