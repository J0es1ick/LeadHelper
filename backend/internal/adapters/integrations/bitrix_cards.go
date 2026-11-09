package integrations

import (
	"encoding/json"
	"html"
	"leadhelper/internal/domain"
	"net/url"
	"regexp"
	"strings"
)

func cardValue(m map[string]any, key string) any {
	if v, ok := m[key]; ok {
		return v
	}
	return m[strings.ToLower(key)]
}
func cardList(v any) []any {
	if a, ok := v.([]any); ok {
		return a
	}
	if v != nil {
		return []any{v}
	}
	return nil
}
func cardString(m map[string]any, key string) string {
	s := str(cardValue(m, key))
	if len(s) > 60000 {
		return string([]rune(s)[:min(len([]rune(s)), 15000)])
	}
	return s
}
func (b *Bitrix) cardURL(raw string) string {
	origin, _ := url.Parse(b.base())
	u, e := url.Parse(html.UnescapeString(raw))
	if e != nil || origin == nil || raw == "" {
		return ""
	}
	u = origin.ResolveReference(u)
	if (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || strings.Contains(strings.ToLower(u.Path), "/rest/") {
		return ""
	}
	return u.String()
}
func (b *Bitrix) messageCards(m map[string]any) []domain.RemoteCard {
	raw := cardValue(obj(m["params"]), "ATTACH")
	if s, ok := raw.(string); ok && len(s) < 120000 {
		var v any
		if json.Unmarshal([]byte(s), &v) == nil {
			raw = v
		}
	}
	cards := []domain.RemoteCard{}
	budget := 0
	var add func(any, int)
	add = func(v any, depth int) {
		if depth > 4 || len(cards) >= 10 || budget >= 100 {
			return
		}
		if list, ok := v.([]any); ok {
			for _, x := range list {
				add(x, depth+1)
			}
			return
		}
		c := obj(v)
		if c == nil {
			return
		}
		blocks := cardValue(c, "BLOCKS")
		if blocks == nil {
			blocks = cardValue(c, "ITEMS")
		}
		if blocks == nil {
			blocks = []any{c}
		}
		card := domain.RemoteCard{Title: cardString(c, "DESCRIPTION"), Blocks: []domain.RemoteBlock{}}
		for _, x := range cardList(blocks) {
			if budget >= 100 {
				break
			}
			m := obj(x)
			for _, kind := range []string{"MESSAGE", "GRID", "LINK", "USER", "IMAGE", "FILE", "DELIMITER"} {
				value := cardValue(m, kind)
				if value == nil {
					continue
				}
				for _, row := range cardList(value) {
					if budget >= 100 {
						break
					}
					block := domain.RemoteBlock{Kind: strings.ToLower(kind)}
					if kind == "MESSAGE" {
						block.Text = str(row)
					} else if kind != "DELIMITER" {
						r := obj(row)
						block.Title = cardString(r, "NAME")
						block.Text = cardString(r, "VALUE")
						if block.Text == "" {
							block.Text = cardString(r, "DESC")
						}
						if block.Text == "" {
							block.Text = cardString(r, "HTML")
						}
						block.URL = b.cardURL(cardString(r, "LINK"))
						if block.URL == "" {
							block.URL = b.cardURL(cardString(r, "URL"))
						}
					}
					if len(block.Text) > 60000 {
						block.Text = string([]rune(block.Text)[:min(len([]rune(block.Text)), 15000)])
					}
					if kind == "DELIMITER" || block.Text != "" || block.Title != "" || block.URL != "" {
						card.Blocks = append(card.Blocks, block)
						budget++
					}
				}
			}
		}
		if len(card.Blocks) > 0 {
			if card.Title == "" {
				card.Title = "Вложение"
			}
			cards = append(cards, card)
		}
	}
	add(raw, 0)
	return cards
}

var messageLink = regexp.MustCompile(`(?i)(?:https?://[^\s\[\]()<>]+)?/online/\?[^\s\[\]()<>]+`)

func (b *Bitrix) messageReferences(text string) []domain.MessageReference {
	out := []domain.MessageReference{}
	seen := map[string]bool{}
	for _, s := range messageLink.FindAllString(text, 10) {
		raw := safeRemoteURL(b.base(), html.UnescapeString(s))
		u, e := url.Parse(raw)
		if e != nil || raw == "" {
			continue
		}
		dialog, id := u.Query().Get("IM_DIALOG"), u.Query().Get("IM_MESSAGE")
		if !remoteID.MatchString(dialog) || !bitrixFileID.MatchString(id) || seen[dialog+":"+id] {
			continue
		}
		seen[dialog+":"+id] = true
		out = append(out, domain.MessageReference{DialogID: dialog, MessageID: id, URL: raw})
	}
	return out
}
