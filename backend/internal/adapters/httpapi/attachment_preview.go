package httpapi

import (
	"bytes"
	"leadhelper/internal/adapters/filepreview"
	"leadhelper/internal/application"
	"leadhelper/internal/domain"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

func (a *API) previewFile(w http.ResponseWriter, r *http.Request) (domain.Attachment, []byte, bool) {
	m, ok := a.findMessage(w, r)
	if !ok {
		return domain.Attachment{}, nil, false
	}
	for _, file := range m.Attachments {
		if file.ID == r.PathValue("attachment") {
			data, err := a.s.AttachmentBytes(r.Context(), file)
			if err != nil {
				respond(w, nil, err)
				return file, nil, false
			}
			return file, data, true
		}
	}
	respond(w, nil, application.ErrNotFound)
	return domain.Attachment{}, nil, false
}

func (a *API) attachmentPreview(w http.ResponseWriter, r *http.Request) {
	file, data, ok := a.previewFile(w, r)
	if !ok {
		return
	}
	send(w, http.StatusOK, filepreview.Build(file.Name, file.ContentType, data))
}

func (a *API) attachmentContent(w http.ResponseWriter, r *http.Request) {
	file, data, ok := a.previewFile(w, r)
	if !ok {
		return
	}
	kind := filepreview.InlineType(data)
	if r.URL.Query().Get("format") == "docx" {
		if !strings.EqualFold(filepath.Ext(file.Name), ".docx") || filepreview.ValidateDOCX(data) != nil {
			send(w, 415, map[string]string{"error": "Документ повреждён или слишком сложен для предпросмотра"})
			return
		}
		kind = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
		w.Header().Set("Content-Type", kind)
		w.Header().Set("Content-Disposition", "attachment")
		w.Header().Set("Cache-Control", "private, no-store")
		http.ServeContent(w, r, file.Name, time.Time{}, bytes.NewReader(data))
		return
	}
	if kind == "" {
		send(w, http.StatusUnsupportedMediaType, map[string]string{"error": "Этот формат нельзя открыть как изображение или PDF"})
		return
	}
	w.Header().Set("Content-Type", kind)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": file.Name}))
	w.Header().Set("X-Frame-Options", "SAMEORIGIN")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'self'; base-uri 'none'; form-action 'none'")
	http.ServeContent(w, r, file.Name, time.Time{}, bytes.NewReader(data))
}
