package product

import (
	"encoding/xml"
	"fmt"
	"regexp"
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

var idSanitizer = regexp.MustCompile(`[^A-Za-z0-9._-]`)

// dotRunSanitizer catches runs of two or more dots. A single "." is allowed
// through idSanitizer (legitimate IDs may contain one), but a run of dots
// left untouched by idSanitizer could otherwise reconstruct a literal ".."
// once path separators are stripped out around it.
var dotRunSanitizer = regexp.MustCompile(`\.{2,}`)

// sanitizeID strips any character that isn't safe to embed directly in a
// filesystem path segment, so untrusted network-supplied fields (cccc,
// ttaaii, awipsid, id all originate from attributes on an incoming XMPP
// stanza) can't be used for path traversal or to inject path separators.
func sanitizeID(s string) string {
	sanitized := idSanitizer.ReplaceAllString(s, "_")
	return dotRunSanitizer.ReplaceAllString(sanitized, "_")
}

// nwwsProcessorTag is a routing/relay tag NWWS-OI sometimes prepends to the
// id attribute (e.g. "nwws_processor.2953"). It carries no product-identity
// information, so it's replaced with a fixed placeholder rather than kept.
const nwwsProcessorTag = "nwws_processor"

// Filename returns the archive filename for this product, entirely
// lowercase: [cccc]_[ttaaii]-[awipsid].[ddHHMM]_[id].txt
func (p Product) Filename() string {
	ddHHMM := p.Issue.UTC().Format("021504")
	cccc := sanitizeID(p.CCCC)
	ttaaii := sanitizeID(p.TTAAII)
	awipsid := sanitizeID(p.AWIPSID)
	id := sanitizeID(strings.ReplaceAll(p.ID, nwwsProcessorTag, "0000"))
	name := fmt.Sprintf("%s_%s-%s.%s_%s.txt", cccc, ttaaii, awipsid, ddHHMM, id)
	return strings.ToLower(name)
}

// Dir returns the subdirectory (relative to the archive root) this product
// belongs in, lowercase.
func (p Product) Dir() string {
	return strings.ToLower(sanitizeID(p.CCCC))
}
