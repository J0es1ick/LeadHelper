package integrations

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"leadhelper/internal/domain"
	"net"
	"strings"
	"testing"
)

type imapStep struct{ command, response string }

func scriptedIMAP(t *testing.T, steps []imapStep) func(context.Context, string, int) (*imapWire, error) {
	t.Helper()
	return func(ctx context.Context, _ string, _ int) (*imapWire, error) {
		client, server := net.Pipe()
		done := make(chan struct{})
		go func() {
			defer close(done)
			defer server.Close()
			reader := bufio.NewReader(server)
			for _, step := range steps {
				line, e := reader.ReadString('\n')
				if e != nil {
					t.Errorf("fixture read: %v", e)
					return
				}
				tag, command, _ := strings.Cut(strings.TrimSuffix(line, "\r\n"), " ")
				if command != step.command {
					t.Errorf("unexpected command (credentials omitted), step expecting %q", strings.Split(step.command, " ")[0])
					return
				}
				if step.response == "DISCONNECT" {
					return
				}
				response := step.response + tag + " OK complete\r\n"
				if strings.HasPrefix(step.response, "NO ") || strings.HasPrefix(step.response, "BAD ") {
					response = tag + " " + step.response + "\r\n"
				}
				if _, e = io.WriteString(server, response); e != nil {
					t.Errorf("fixture write: %v", e)
					return
				}
			}
		}()
		t.Cleanup(func() { client.Close(); <-done })
		return &imapWire{conn: client, r: bufio.NewReader(client), maxLiteral: 32 << 20, cancel: context.AfterFunc(ctx, func() { client.Close() })}, nil
	}
}
func loginSteps() []imapStep {
	return []imapStep{{`LOGIN "fixture@example.test" "fixture-password"`, ""}, {`LIST "" "*"`, "* LIST (\\HasNoChildren) \"/\" \"INBOX\"\r\n* LIST (\\Noselect) \"/\" \"Virtual\"\r\n"}}
}
func selection(epoch, after int, search string) []imapStep {
	return []imapStep{{`EXAMINE "INBOX"`, fmt.Sprintf("* OK [UIDVALIDITY %d] epoch\r\n", epoch)}, {fmt.Sprintf("UID SEARCH UID %d:*", after), "* SEARCH " + search + "\r\n"}}
}
func fetchSteps(uid int, raw string) []imapStep {
	return []imapStep{
		{fmt.Sprintf("UID FETCH %d (UID RFC822.SIZE FLAGS)", uid), fmt.Sprintf("* 1 FETCH (UID %d RFC822.SIZE %d FLAGS (\\Seen))\r\n* 2 FETCH (UID 999 RFC822.SIZE 4 FLAGS ())\r\n", uid, len(raw))},
		{fmt.Sprintf("UID FETCH %d (UID BODY.PEEK[])", uid), fmt.Sprintf("* 1 FETCH (UID %d BODY[] {%d}\r\n%s)\r\n", uid, len(raw), raw)},
	}
}
func TestIMAPResumeAfterDisconnectAndUIDValidityReset(t *testing.T) {
	raw := "From: Fixture <a@example.test>\r\nTo: fixture@example.test\r\nSubject: Test\r\nMessage-ID: <sample@example.test>\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nhello ) ( quoted \" body\r\n"
	imap := &IMAP{Host: "fixture.test", User: "fixture@example.test", Password: "fixture-password", BatchSize: 1}
	cursors := map[string]domain.MailCheckpoint{}
	messages := []domain.Message{}
	commit := func(b domain.MailBatch) error {
		cursors[b.Key] = b.Checkpoint
		messages = append(messages, b.Messages...)
		return nil
	}
	steps := append(loginSteps(), selection(70, 1, "1 2")...)
	steps = append(steps, fetchSteps(1, raw)...)
	steps = append(steps, imapStep{"UID FETCH 2 (UID RFC822.SIZE FLAGS)", "DISCONNECT"})
	imap.Dial = scriptedIMAP(t, steps)
	if e := imap.Sync(context.Background(), cursors, commit); e == nil {
		t.Fatal("disconnect must fail")
	}
	if len(messages) != 1 || messages[0].UID != 1 || !messages[0].Read || messages[0].MessageID != "<sample@example.test>" {
		t.Fatalf("first committed batch: %+v", messages)
	}
	steps = append(loginSteps(), selection(70, 2, "1 2")...)
	steps = append(steps, fetchSteps(2, raw)...)
	imap.Dial = scriptedIMAP(t, steps)
	if e := imap.Sync(context.Background(), cursors, commit); e != nil {
		t.Fatal(e)
	}
	if len(messages) != 2 || messages[1].UID != 2 {
		t.Fatal("resume duplicated/skipped UID")
	}
	// Reversed ranges may include the highest existing UID when no newer mail exists.
	steps = append(loginSteps(), selection(70, 3, "2")...)
	imap.Dial = scriptedIMAP(t, steps)
	if e := imap.Sync(context.Background(), cursors, commit); e != nil {
		t.Fatal(e)
	}
	if len(messages) != 2 {
		t.Fatal("reversed star range duplicated mail")
	}
	steps = append(loginSteps(), selection(71, 1, "1")...)
	steps = append(steps, fetchSteps(1, raw)...)
	imap.Dial = scriptedIMAP(t, steps)
	if e := imap.Sync(context.Background(), cursors, commit); e != nil {
		t.Fatal(e)
	}
	if messages[2].UIDValidity != 71 {
		t.Fatal("UIDVALIDITY change ignored")
	}
}
func TestIMAPMalformedMIMEPreservesOriginal(t *testing.T) {
	raw := "this is not an RFC message\r\n"
	steps := append(loginSteps(), selection(10, 1, "1")...)
	steps = append(steps, fetchSteps(1, raw)...)
	imap := &IMAP{Host: "fixture.test", User: "fixture@example.test", Password: "fixture-password", Dial: scriptedIMAP(t, steps)}
	e := imap.Sync(context.Background(), nil, func(b domain.MailBatch) error {
		if len(b.Messages) != 1 || b.Messages[0].ImportWarning == "" || string(b.Messages[0].Raw) != raw {
			t.Fatal("raw malformed message was lost")
		}
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
}
func TestIMAPParserBoundsAndModifiedUTF7(t *testing.T) {
	for _, raw := range []string{"* X {9999999999999999999999999999999999999999999999}\r\n", "* X {33554433}\r\n", "* X (unterminated\r\n"} {
		w := imapWire{r: bufio.NewReader(strings.NewReader(raw)), maxLiteral: 32 << 20}
		if _, e := w.response(); e == nil {
			t.Fatal("malformed input accepted")
		}
	}
	if got := decodeMailbox("&BB8EQAQ4BDIENQRC- &- Team"); got != "Привет & Team" {
		t.Fatalf("UTF7 %q", got)
	}
	if _, e := imapQuote("user\r\nLOGOUT"); e == nil {
		t.Fatal("command injection accepted")
	}
}

func TestIMAPDeniedFolderDoesNotBlockOtherFolders(t *testing.T) {
	for _, rejected := range []string{`EXAMINE "INBOX"`, "UID FETCH 1 (UID RFC822.SIZE FLAGS)"} {
		t.Run(rejected, func(t *testing.T) {
			steps := loginSteps()
			steps[1].response += "* LIST (\\Sent) \"/\" \"Sent\"\r\n"
			if strings.HasPrefix(rejected, "UID FETCH") {
				steps = append(steps, selection(10, 1, "1")...)
			}
			steps = append(steps, imapStep{rejected, "NO permission denied"})
			steps = append(steps, imapStep{`EXAMINE "Sent"`, "* OK [UIDVALIDITY 20] epoch\r\n"}, imapStep{"UID SEARCH UID 1:*", "* SEARCH 5\r\n"})
			steps = append(steps, fetchSteps(5, "Subject: Sent message\r\n\r\nhello")...)
			imap := &IMAP{Host: "fixture.test", User: "fixture@example.test", Password: "fixture-password", Dial: scriptedIMAP(t, steps)}
			var batches []domain.MailBatch
			err := imap.Sync(context.Background(), nil, func(b domain.MailBatch) error {
				batches = append(batches, b)
				return nil
			})
			if err == nil || !strings.Contains(err.Error(), "INBOX") {
				t.Fatalf("partial sync must identify the failed folder: %v", err)
			}
			if len(batches) != 1 || batches[0].Checkpoint.Folder != "Sent" || batches[0].Checkpoint.LastUID != 5 || len(batches[0].Messages) != 1 {
				t.Fatalf("other folder was not saved or rejected folder advanced: %+v", batches)
			}
		})
	}
}

func TestIMAPCommitFailureStopsSync(t *testing.T) {
	steps := loginSteps()
	steps[1].response += "* LIST (\\Sent) \"/\" \"Sent\"\r\n"
	steps = append(steps, selection(10, 1, "1")...)
	steps = append(steps, fetchSteps(1, "Subject: Test\r\n\r\nbody")...)
	imap := &IMAP{Host: "fixture.test", User: "fixture@example.test", Password: "fixture-password", Dial: scriptedIMAP(t, steps)}
	want := errors.New("disk unavailable")
	err := imap.Sync(context.Background(), nil, func(domain.MailBatch) error { return want })
	if !errors.Is(err, want) {
		t.Fatalf("storage failure must stop synchronization: %v", err)
	}
}
