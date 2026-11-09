package storage

import (
	"context"
	"encoding/json"
	"leadhelper/internal/domain"
)

// Only indexed notification targets (at most 200), never the entire mailbox.
// This also retires old notifications for messages read before this upgrade.
func reconcileMailNotifications(ctx context.Context, q queryer, st *domain.State) error {
	targets := []string{}
	for _, n := range st.Notifications {
		if n.Provider == "mail" && !n.Read {
			targets = append(targets, n.Target)
		}
	}
	if len(targets) == 0 {
		return nil
	}
	data, e := json.Marshal(targets)
	if e != nil {
		return e
	}
	rows, e := q.QueryContext(ctx, `SELECT m.id FROM json_each(?) n JOIN messages m ON m.id=n.value WHERE m.is_read=1`, string(data))
	if e != nil {
		return e
	}
	defer rows.Close()
	read := map[string]bool{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			return e
		}
		read[id] = true
	}
	if e = rows.Err(); e != nil {
		return e
	}
	for i := range st.Notifications {
		if st.Notifications[i].Provider == "mail" && read[st.Notifications[i].Target] {
			st.Notifications[i].Read = true
		}
	}
	return nil
}
