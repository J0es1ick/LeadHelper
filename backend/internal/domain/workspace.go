package domain

import "time"

// Remote collections are paged and fetched on demand, never added to /state.
type RemoteItem struct {
	AvatarURL     string             `json:"avatarUrl,omitempty"`
	HeadAvatarURL string             `json:"headAvatarUrl,omitempty"`
	System        bool               `json:"system,omitempty"`
	Cards         []RemoteCard       `json:"cards,omitempty"`
	References    []MessageReference `json:"references,omitempty"`
	Attachments   []RemoteAttachment `json:"attachments,omitempty"`
	Status        string             `json:"status,omitempty"`
	Online        bool               `json:"online,omitempty"`
	Departments   []string           `json:"departments,omitempty"`
	HeadID        string             `json:"headId,omitempty"`
	HeadName      string             `json:"headName,omitempty"`
	HeadURL       string             `json:"headUrl,omitempty"`
	Sort          int                `json:"sort,omitempty"`
	ID            string             `json:"id"`
	Title         string             `json:"title"`
	Text          string             `json:"text,omitempty"`
	HTML          string             `json:"html,omitempty"`
	URL           string             `json:"url,omitempty"`
	Kind          string             `json:"kind,omitempty"`
	Parent        string             `json:"parent,omitempty"`
	Author        string             `json:"author,omitempty"`
	Date          time.Time          `json:"date,omitempty"`
	Unread        int                `json:"unread,omitempty"`
	MessageID     string             `json:"messageId,omitempty"`
	ReadMessageID string             `json:"readMessageId,omitempty"`
	Own           bool               `json:"own,omitempty"`
}
type MessageReference struct {
	DialogID  string `json:"dialogId"`
	MessageID string `json:"messageId"`
	URL       string `json:"url"`
}
type RemoteCard struct {
	Title  string        `json:"title"`
	Blocks []RemoteBlock `json:"blocks"`
}
type RemoteBlock struct {
	Kind  string `json:"kind"`
	Title string `json:"title,omitempty"`
	Text  string `json:"text,omitempty"`
	URL   string `json:"url,omitempty"`
}
type RemoteAttachment struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Size  int64  `json:"size"`
	Image bool   `json:"image"`
	URL   string `json:"url"`
}
type RemotePage struct {
	AccountID string       `json:"accountId,omitempty"`
	Total     *int         `json:"total,omitempty"`
	TotalAll  *int         `json:"totalAll,omitempty"`
	Warning   string       `json:"warning,omitempty"`
	Items     []RemoteItem `json:"items"`
	Next      string       `json:"next,omitempty"`
}
type ChatRead struct {
	MessageID string `json:"messageId"`
	Unread    int    `json:"unread"`
}
type RemoteQuery struct{ Kind, ID, Search, Cursor, Status, Department string }
type CalendarEvent struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Start       time.Time `json:"start"`
	End         time.Time `json:"end"`
	AllDay      bool      `json:"allDay"`
	Busy        bool      `json:"busy"`
	Tentative   bool      `json:"tentative"`
	Location    string    `json:"location,omitempty"`
	Description string    `json:"description,omitempty"`
	URL         string    `json:"url"`
}
type CalendarPage struct {
	Events    []CalendarEvent `json:"events"`
	UpdatedAt time.Time       `json:"updatedAt"`
	Timezone  string          `json:"timezone"`
}
type Notification struct {
	ID       string    `json:"id"`
	Provider string    `json:"provider"`
	Title    string    `json:"title"`
	Text     string    `json:"text"`
	Target   string    `json:"target"`
	At       time.Time `json:"at"`
	Read     bool      `json:"read"`
}
type IntegrationConfig struct {
	Scope string `json:"scope"`
	URL   string `json:"url"`
	Token string `json:"token"`
	Email string `json:"email"`
	Mode  string `json:"mode"`
	JQL   string `json:"jql"`
}
