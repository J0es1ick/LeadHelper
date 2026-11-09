// Package bitrixtext turns provider formatting into readable notification text.
// Full message bodies stay intact for the frontend's safe rich-text renderer.
package bitrixtext

import (
	"html"
	"regexp"
	"strconv"
	"strings"
)

var code = regexp.MustCompile(`(?is)\[CODE(?:[ =][^\]\r\n]*)?\](.*?)(?:\[/CODE\]|$)`)
var tag = regexp.MustCompile(`(?i)\[(/?)([a-z][a-z0-9_-]*|\*)([^\]\r\n]*)\]`)
var htmlTag = regexp.MustCompile(`(?i)</?(?:p|div|br|span|b|strong|i|em|u|s|a|img|blockquote|ul|ol|li|table|tbody|tr|td|th)\b[^>]*>`)
var script = regexp.MustCompile(`(?is)<(?:script|style)\b[^>]*>.*?</(?:script|style)\s*>`)
var spaces = regexp.MustCompile(`\n{3,}`)
var title = regexp.MustCompile(`(?i)(?:^|\s)title\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s]+))`)

const known = " B I U S P QUOTE CODE LIST IMG URL SIZE COLOR FONT CENTER LEFT RIGHT JUSTIFY TABLE TR TD TH USER CHAT CONTEXT SEND PUT CALL BR ICON TIMESTAMP DISK FILE ATTACH ENTITY MENTION COMMAND * "

func Plain(source string) string {
	codes := []string{}
	text := code.ReplaceAllStringFunc(source, func(value string) string {
		codes = append(codes, code.FindStringSubmatch(value)[1])
		return "\x00" + strconv.Itoa(len(codes)-1) + "\x00"
	})
	text = script.ReplaceAllString(text, "")
	text = htmlTag.ReplaceAllStringFunc(text, func(value string) string {
		v := strings.ToLower(value)
		if strings.HasPrefix(v, "<br") || v == "</p>" || v == "</div>" || v == "</li>" || v == "</tr>" {
			return "\n"
		}
		return ""
	})
	text = tag.ReplaceAllStringFunc(text, func(value string) string {
		m := tag.FindStringSubmatch(value)
		name := strings.ToUpper(m[2])
		if !strings.Contains(known, " "+name+" ") {
			return value
		}
		if m[1] == "/" {
			if strings.Contains(" P QUOTE LIST CENTER LEFT RIGHT JUSTIFY TABLE TR ", " "+name+" ") {
				return "\n"
			}
			return ""
		}
		switch name {
		case "BR":
			return "\n"
		case "*":
			return "\n• "
		case "DISK", "FILE", "ATTACH":
			return "📎 Вложение"
		case "ICON":
			if fields := title.FindStringSubmatch(m[3]); fields != nil {
				for _, field := range fields[1:] {
					if field != "" {
						return field
					}
				}
			}
		}
		return ""
	})
	text = html.UnescapeString(text)
	for i, value := range codes {
		text = strings.ReplaceAll(text, "\x00"+strconv.Itoa(i)+"\x00", value)
	}
	return strings.TrimSpace(spaces.ReplaceAllString(text, "\n\n"))
}
