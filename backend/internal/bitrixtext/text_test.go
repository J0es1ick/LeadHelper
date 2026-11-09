package bitrixtext

import "testing"

func TestServiceMarkupKeepsReadableContent(t *testing.T) {
	for source, want := range map[string]string{
		`[USER=726 REPLACE]Евгений Крылов[/USER] пригласил в чат [USER=5120 REPLACE]Дмитрий Нарзуллаев[/USER], [USER=7752 REPLACE]Игорь Павлов[/USER]`: "Евгений Крылов пригласил в чат Дмитрий Нарзуллаев, Игорь Павлов",
		`[user=all replace][B]Все[/B][/user] [CHAT=42 REPLACE]Команда[/CHAT]`:                                                                          "Все Команда",
		`[CONTEXT=chat42/99]Ответ[/CONTEXT] [URL=https://example.test]План[/URL]`:                                                                      "Ответ План",
		`<p>Текст &amp; <strong>оформление</strong></p><script>bad()</script>`:                                                                         "Текст & оформление",
		`[Важно] [CODE]const x = [USER=1]; <b>literal</b>[/CODE]`:                                                                                      "[Важно] const x = [USER=1]; <b>literal</b>",
		`[ICON=https://example.test/a.png title="Готово" size=20] [DISK FILE ID=123]`:                                                                  "Готово 📎 Вложение",
	} {
		if got := Plain(source); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
}
