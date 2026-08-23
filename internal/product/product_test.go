package product

import (
	"encoding/xml"
	"strings"
	"testing"
	"time"

	"gosrc.io/xmpp/stanza"
)

const validMessageXML = `<message from="nwws@conference.nwws-oi.weather.gov/KKCI" to="user@nwws-oi.weather.gov/res">
  <x xmlns="nwws-oi" cccc="KKCI" ttaaii="FTUS21" awipsid="TAFKORD" issue="2026-08-22T14:32:00Z" id="12345">SAMPLE PRODUCT TEXT</x>
</message>`

func unmarshalMessage(t *testing.T, xmlStr string) stanza.Message {
	t.Helper()
	var msg stanza.Message
	if err := xml.Unmarshal([]byte(xmlStr), &msg); err != nil {
		t.Fatalf("unmarshaling test message: %v", err)
	}
	return msg
}

func TestParseMessage_Valid(t *testing.T) {
	msg := unmarshalMessage(t, validMessageXML)

	p, err := ParseMessage(msg)
	if err != nil {
		t.Fatalf("ParseMessage: %v", err)
	}

	if p.CCCC != "KKCI" {
		t.Errorf("CCCC = %q, want KKCI", p.CCCC)
	}
	if p.TTAAII != "FTUS21" {
		t.Errorf("TTAAII = %q, want FTUS21", p.TTAAII)
	}
	if p.AWIPSID != "TAFKORD" {
		t.Errorf("AWIPSID = %q, want TAFKORD", p.AWIPSID)
	}
	if p.ID != "12345" {
		t.Errorf("ID = %q, want 12345", p.ID)
	}
	if strings.TrimSpace(p.Text) != "SAMPLE PRODUCT TEXT" {
		t.Errorf("Text = %q, want SAMPLE PRODUCT TEXT", p.Text)
	}
	wantIssue := time.Date(2026, 8, 22, 14, 32, 0, 0, time.UTC)
	if !p.Issue.Equal(wantIssue) {
		t.Errorf("Issue = %v, want %v", p.Issue, wantIssue)
	}
}

func TestParseMessage_NoExtension(t *testing.T) {
	msg := unmarshalMessage(t, `<message><body>just a chat message</body></message>`)

	_, err := ParseMessage(msg)
	if err == nil {
		t.Fatal("ParseMessage: expected error for a message with no nwws-oi extension")
	}
}

func TestParseMessage_MissingAttribute(t *testing.T) {
	xmlStr := `<message><x xmlns="nwws-oi" ttaaii="FTUS21" awipsid="TAFKORD" issue="2026-08-22T14:32:00Z" id="12345">TEXT</x></message>`
	msg := unmarshalMessage(t, xmlStr)

	_, err := ParseMessage(msg)
	if err == nil {
		t.Fatal("ParseMessage: expected error for missing cccc attribute")
	}
	if !strings.Contains(err.Error(), "cccc") {
		t.Errorf("error %q does not mention the missing field cccc", err.Error())
	}
}

func TestParseMessage_InvalidIssueTimestamp(t *testing.T) {
	xmlStr := `<message><x xmlns="nwws-oi" cccc="KKCI" ttaaii="FTUS21" awipsid="TAFKORD" issue="not-a-timestamp" id="12345">TEXT</x></message>`
	msg := unmarshalMessage(t, xmlStr)

	_, err := ParseMessage(msg)
	if err == nil {
		t.Fatal("ParseMessage: expected error for invalid issue timestamp")
	}
}

func TestProduct_Filename(t *testing.T) {
	p := Product{
		CCCC:    "KKCI",
		TTAAII:  "FTUS21",
		AWIPSID: "TAFKORD",
		Issue:   time.Date(2026, 8, 22, 14, 32, 0, 0, time.UTC),
		ID:      "12345",
	}

	got := p.Filename()
	want := "kkci_ftus21-tafkord.221432_12345.txt"
	if got != want {
		t.Errorf("Filename() = %q, want %q", got, want)
	}
}

func TestProduct_Filename_ReplacesNwwsProcessorInID(t *testing.T) {
	p := Product{
		CCCC:    "KKCI",
		TTAAII:  "FTUS21",
		AWIPSID: "TAFKORD",
		Issue:   time.Date(2026, 8, 22, 14, 32, 0, 0, time.UTC),
		ID:      "nwws_processor.2953",
	}

	got := p.Filename()
	want := "kkci_ftus21-tafkord.221432_0000.2953.txt"
	if got != want {
		t.Errorf("Filename() = %q, want %q", got, want)
	}
}

func TestProduct_Filename_SanitizesID(t *testing.T) {
	p := Product{
		CCCC:    "KKCI",
		TTAAII:  "FTUS21",
		AWIPSID: "TAFKORD",
		Issue:   time.Date(2026, 8, 22, 14, 32, 0, 0, time.UTC),
		ID:      "abc/def ghi",
	}

	got := p.Filename()
	if strings.ContainsAny(got, "/ ") {
		t.Errorf("Filename() = %q, contains unsafe characters from the raw ID", got)
	}
}

func TestProduct_Dir(t *testing.T) {
	p := Product{CCCC: "KKCI"}
	if p.Dir() != "kkci" {
		t.Errorf("Dir() = %q, want kkci", p.Dir())
	}
}

func TestProduct_SanitizesCCCC(t *testing.T) {
	p := Product{
		CCCC:    "../../../../tmp/pwned",
		TTAAII:  "FTUS21",
		AWIPSID: "TAFKORD",
		Issue:   time.Date(2026, 8, 22, 14, 32, 0, 0, time.UTC),
		ID:      "12345",
	}

	dir := p.Dir()
	if strings.Contains(dir, "/") || strings.Contains(dir, "..") {
		t.Errorf("Dir() = %q, contains unsafe path traversal characters from the raw CCCC", dir)
	}

	filename := p.Filename()
	if strings.Contains(filename, "/") || strings.Contains(filename, "..") {
		t.Errorf("Filename() = %q, contains unsafe path traversal characters from the raw CCCC", filename)
	}
}
