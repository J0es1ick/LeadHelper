package domain

type MailScope struct {
	IDs   []string
	Batch []Message
}
type MailQuery struct {
	Folder, Search, Cursor string
	Limit                  int
}
type MailThread struct {
	ID             string  `json:"id"`
	Latest         Message `json:"latest"`
	Count          int     `json:"count"`
	Unread         int     `json:"unread"`
	Starred        bool    `json:"starred"`
	HasAttachments bool    `json:"hasAttachments"`
}
type MailPage struct {
	Items      []MailThread `json:"items"`
	NextCursor string       `json:"nextCursor,omitempty"`
	Limit      int          `json:"limit"`
}
type MessagePage struct {
	Items      []Message  `json:"items"`
	NextCursor string     `json:"nextCursor,omitempty"`
	Thread     MailThread `json:"thread"`
}
type MailStats struct {
	Folders  map[string]int `json:"folders"`
	Unread   int            `json:"unread"`
	Revision int64          `json:"revision"`
}
type MailPatch struct {
	Folder  string `json:"folder"`
	Read    *bool  `json:"read"`
	Starred *bool  `json:"starred"`
}
