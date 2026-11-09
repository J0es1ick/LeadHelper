package domain

const ChatFileLimit = 25 << 20

type ChatUpload struct {
	Name, Text, AccountID, RequestID string
	Data                             []byte
}
type ChatUploadResult struct {
	MessageID string `json:"messageId"`
	FileID    string `json:"fileId"`
}
