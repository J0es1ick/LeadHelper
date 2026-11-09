package integrations

import "strings"

// Reuse the image URLs already returned with chats/users: no per-message lookup.
// Only the configured portal and Bitrix CDN are accepted, never REST credentials.
func (b *Bitrix) avatarURL(values ...any) string {
	for _, value := range values {
		raw := strings.TrimSpace(str(value))
		if raw == "" {
			raw = str(obj(value)["url"])
		}
		if !strings.HasPrefix(raw, "/") && !strings.HasPrefix(raw, "https://") {
			continue
		}
		u, e := b.downloadURL(raw)
		if e != nil || strings.Contains(strings.ToLower(u.Path), "/rest/") {
			continue
		}
		secret := false
		for key := range u.Query() {
			switch strings.ToLower(key) {
			case "auth", "auth_id", "access_token", "refresh_token":
				secret = true
			}
		}
		if !secret {
			return u.String()
		}
	}
	return ""
}
