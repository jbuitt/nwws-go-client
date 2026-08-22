package product

import (
	"encoding/xml"
	"fmt"
	"strings"
	"time"

	"gosrc.io/xmpp/stanza"
)

// nwwsExtension is the <x xmlns="nwws-oi"> child element carrying product
// metadata and the raw product text on a MUC message stanza.
type nwwsExtension struct {
	stanza.MsgExtension
	XMLName xml.Name `xml:"nwws-oi x"`
	CCCC    string   `xml:"cccc,attr"`
	TTAAII  string   `xml:"ttaaii,attr"`
	AWIPSID string   `xml:"awipsid,attr"`
	Issue   string   `xml:"issue,attr"`
	ID      string   `xml:"id,attr"`
	Text    string   `xml:",chardata"`
}

func init() {
	// go-xmpp silently drops any message sub-element it doesn't recognize,
	// so the nwws-oi extension must be registered before any message is
	// parsed, or product data is lost with no error.
	stanza.TypeRegistry.MapExtension(stanza.PKTMessage, xml.Name{Space: "nwws-oi", Local: "x"}, nwwsExtension{})
}

// Product is a parsed NWWS-OI weather product, ready to be filed to disk.
type Product struct {
	CCCC    string
	TTAAII  string
	AWIPSID string
	Issue   time.Time
	ID      string
	Text    string
}

// ParseMessage extracts a Product from an XMPP message stanza carrying an
// nwws-oi extension. It returns an error describing what's missing or
// invalid if the message doesn't contain a usable product.
func ParseMessage(msg stanza.Message) (Product, error) {
	var ext nwwsExtension
	if !msg.Get(&ext) {
		return Product{}, fmt.Errorf("message has no nwws-oi extension")
	}

	var missing []string
	if ext.CCCC == "" {
		missing = append(missing, "cccc")
	}
	if ext.TTAAII == "" {
		missing = append(missing, "ttaaii")
	}
	if ext.AWIPSID == "" {
		missing = append(missing, "awipsid")
	}
	if ext.Issue == "" {
		missing = append(missing, "issue")
	}
	if ext.ID == "" {
		missing = append(missing, "id")
	}
	if len(missing) > 0 {
		return Product{}, fmt.Errorf("nwws-oi product missing required attribute(s): %s", strings.Join(missing, ", "))
	}

	issue, err := time.Parse(time.RFC3339, ext.Issue)
	if err != nil {
		return Product{}, fmt.Errorf("nwws-oi product has invalid issue timestamp %q: %w", ext.Issue, err)
	}

	return Product{
		CCCC:    ext.CCCC,
		TTAAII:  ext.TTAAII,
		AWIPSID: ext.AWIPSID,
		Issue:   issue.UTC(),
		ID:      ext.ID,
		Text:    ext.Text,
	}, nil
}
