package httpapi

import (
	"errors"
	"io"
	"leadhelper/internal/application"
	"leadhelper/internal/domain"
	"net/http"
)

func (a *API) bitrixUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, domain.ChatFileLimit+(64<<10))
	if e := r.ParseMultipartForm(1 << 20); e != nil {
		send(w, 400, map[string]any{"error": "Не удалось прочитать файл. Максимальный размер — 25 МБ", "uncertain": false})
		return
	}
	defer r.MultipartForm.RemoveAll()
	if len(r.MultipartForm.File) != 1 || len(r.MultipartForm.File["file"]) != 1 {
		send(w, 400, map[string]string{"error": "Выберите один файл для загрузки"})
		return
	}
	part, header, e := r.FormFile("file")
	if e != nil {
		send(w, 400, map[string]string{"error": "Файл не получен"})
		return
	}
	defer part.Close()
	data, e := io.ReadAll(io.LimitReader(part, domain.ChatFileLimit+1))
	if e != nil || len(data) > domain.ChatFileLimit {
		send(w, 400, map[string]string{"error": "Не удалось прочитать файл до 25 МБ"})
		return
	}
	result, e := a.s.SendChatFile(r.Context(), r.PathValue("id"), domain.ChatUpload{Name: header.Filename, Data: data, Text: r.FormValue("text"), AccountID: r.FormValue("accountId"), RequestID: r.FormValue("requestId")})
	var unconfirmed application.ChatUploadUnconfirmed
	if errors.As(e, &unconfirmed) {
		send(w, 502, map[string]any{"error": e.Error(), "uncertain": true})
		return
	}
	respond(w, result, e)
}
