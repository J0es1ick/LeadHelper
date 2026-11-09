/* Local, sandboxed DOCX renderer. No document content leaves the browser. */
(() => {
  let started = false,
    pages = [],
    page = 0,
    zoom = 100;
  const post = (type, extra = {}) =>
    parent.postMessage({ type: "leadhelper:docx:" + type, ...extra }, "*");
  function show() {
    pages.forEach((p, i) => {
      p.style.display = i === page ? "" : "none";
      p.style.zoom = String(zoom / 100);
    });
    window.scrollTo(0, 0);
  }
  async function render(bytes) {
    try {
      const zip = await JSZip.loadAsync(bytes);
      // Never resolve linked images, attached HTML, macros or embedded objects.
      for (const [name, file] of Object.entries(zip.files)) {
        if (/\.rels$/i.test(name)) {
          const xml = new DOMParser().parseFromString(
            await file.async("string"),
            "application/xml",
          );
          if (xml.querySelector("parsererror"))
            throw Error("Некорректные связи документа");
          for (const rel of [
            ...xml.getElementsByTagNameNS("*", "Relationship"),
          ]) {
            const target = rel.getAttribute("Target") || "",
              type = rel.getAttribute("Type") || "";
            if (
              rel.getAttribute("TargetMode") === "External" ||
              /^(?:[a-z]+:|\/\/)/i.test(target) ||
              /aFChunk|oleObject|vbaProject|package$/i.test(type)
            )
              rel.remove();
          }
          zip.file(name, new XMLSerializer().serializeToString(xml));
        }
        if (/vbaProject|\/embeddings\/|\.html?$/i.test(name)) zip.remove(name);
      }
      const safe = await zip.generateAsync({ type: "arraybuffer" });
      const root = document.getElementById("pages");
      await docx.renderAsync(safe, root, document.getElementById("styles"), {
        className: "docx",
        inWrapper: true,
        breakPages: true,
        ignoreLastRenderedPageBreak: false,
        renderHeaders: true,
        renderFooters: true,
        renderFootnotes: true,
        renderEndnotes: true,
        renderAltChunks: false,
        renderChanges: false,
        renderComments: false,
        ignoreFonts: false,
        useBase64URL: true,
      });
      root
        .querySelectorAll("script,iframe,object,embed,form,link,meta")
        .forEach((el) => el.remove());
      root.querySelectorAll("*").forEach((el) => {
        for (const attr of [...el.attributes])
          if (
            /^on/i.test(attr.name) ||
            ["href", "xlink:href", "srcdoc", "formaction"].includes(attr.name)
          )
            el.removeAttribute(attr.name);
      });
      pages = [...root.querySelectorAll("section.docx")];
      if (!pages.length) throw Error("Документ не содержит доступных страниц");
      const width = pages[0].getBoundingClientRect().width;
      zoom = Math.min(
        100,
        Math.max(25, Math.floor(((innerWidth - 48) / width) * 100)),
      );
      show();
      post("rendered", { pages: pages.length, zoom });
    } catch (e) {
      post("error", {
        message:
          "Не удалось отобразить документ с оформлением. Можно переключиться на текст или скачать оригинал.",
      });
    }
  }
  addEventListener("message", (event) => {
    if (event.source !== parent) return;
    const data = event.data;
    if (data?.type === "leadhelper:docx:hello") post("ready");
    if (
      data?.type === "leadhelper:docx:load" &&
      !started &&
      data.bytes instanceof ArrayBuffer
    ) {
      started = true;
      void render(data.bytes);
    }
    if (data?.type === "leadhelper:docx:view" && pages.length) {
      page = Math.max(0, Math.min(pages.length - 1, Number(data.page) || 0));
      zoom = Math.max(25, Math.min(200, Number(data.zoom) || 100));
      show();
    }
  });
  post("ready");
})();
