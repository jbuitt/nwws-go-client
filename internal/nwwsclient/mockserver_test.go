package nwwsclient

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/xml"
	"fmt"
	"io"
	"math/big"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// connRecord captures what one accepted connection did, so tests can assert
// on the protocol the client actually spoke.
type connRecord struct {
	startTLS   bool // client negotiated STARTTLS
	authSeen   bool // client sent SASL <auth>
	authSecure bool // ...and did so over TLS
	joined     bool // client sent the MUC join presence
}

// mockXMPP is a minimal XMPP server: optional-TLS Openfire-style features,
// STARTTLS, SASL PLAIN, resource bind. It delivers one nwws-oi product per
// connection after the MUC join, then drops connection 0 to simulate the
// server closing a long-lived session.
type mockXMPP struct {
	ln   net.Listener
	cert tls.Certificate

	// stall lists connection indexes the server accepts but never answers,
	// simulating a server that goes quiet mid-handshake. Read-only after
	// construction.
	stall map[int]bool

	mu    sync.Mutex
	conns []*connRecord
}

// newMockXMPP starts a server; any stallConns indexes are accepted but never
// answered.
func newMockXMPP(t *testing.T, stallConns ...int) *mockXMPP {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	m := &mockXMPP{ln: ln, cert: selfSignedCert(t), stall: map[int]bool{}}
	for _, i := range stallConns {
		m.stall[i] = true
	}
	t.Cleanup(func() { ln.Close() })
	go m.serve()
	return m
}

func (m *mockXMPP) port() int { return m.ln.Addr().(*net.TCPAddr).Port }

func (m *mockXMPP) records() []connRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]connRecord, len(m.conns))
	for i, r := range m.conns {
		out[i] = *r
	}
	return out
}

func (m *mockXMPP) update(rec *connRecord, fn func(*connRecord)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	fn(rec)
}

func (m *mockXMPP) serve() {
	for {
		nc, err := m.ln.Accept()
		if err != nil {
			return
		}
		m.mu.Lock()
		rec := &connRecord{}
		m.conns = append(m.conns, rec)
		idx := len(m.conns) - 1
		m.mu.Unlock()
		go m.handle(nc, rec, idx)
	}
}

const (
	nsTLS  = "urn:ietf:params:xml:ns:xmpp-tls"
	nsSASL = "urn:ietf:params:xml:ns:xmpp-sasl"
	nsBind = "urn:ietf:params:xml:ns:xmpp-bind"
	nsSess = "urn:ietf:params:xml:ns:xmpp-session"
)

func (m *mockXMPP) handle(nc net.Conn, rec *connRecord, idx int) {
	defer nc.Close()
	if m.stall[idx] {
		io.Copy(io.Discard, nc) // read and ignore until the client hangs up
		return
	}
	var conn net.Conn = nc
	dec := xml.NewDecoder(conn)
	secure, authed := false, false

	for {
		tok, err := dec.Token()
		if err != nil {
			return
		}
		switch el := tok.(type) {
		case xml.EndElement:
			if el.Name.Local == "stream" { // client closing the stream
				io.WriteString(conn, "</stream:stream>")
				return
			}
		case xml.StartElement:
			switch el.Name.Local {
			case "stream":
				fmt.Fprintf(conn, `<?xml version='1.0' encoding='UTF-8'?><stream:stream xmlns:stream="http://etherx.jabber.org/streams" xmlns="jabber:client" from="127.0.0.1" id="s%d" version="1.0">`, idx)
				switch {
				case authed:
					fmt.Fprintf(conn, `<stream:features><bind xmlns="%s"/><session xmlns="%s"><optional/></session></stream:features>`, nsBind, nsSess)
				case secure:
					fmt.Fprintf(conn, `<stream:features><mechanisms xmlns="%s"><mechanism>PLAIN</mechanism></mechanisms></stream:features>`, nsSASL)
				default:
					// Optional TLS, like the real server: STARTTLS and SASL
					// both advertised on the plaintext stream.
					fmt.Fprintf(conn, `<stream:features><starttls xmlns="%s"></starttls><mechanisms xmlns="%s"><mechanism>PLAIN</mechanism></mechanisms></stream:features>`, nsTLS, nsSASL)
				}
			case "starttls":
				dec.Skip()
				io.WriteString(conn, `<proceed xmlns="`+nsTLS+`"/>`)
				tc := tls.Server(nc, &tls.Config{Certificates: []tls.Certificate{m.cert}})
				if err := tc.Handshake(); err != nil {
					return
				}
				conn, dec, secure = tc, xml.NewDecoder(tc), true
				m.update(rec, func(r *connRecord) { r.startTLS = true })
			case "auth":
				dec.Skip()
				m.update(rec, func(r *connRecord) { r.authSeen, r.authSecure = true, secure })
				if !secure {
					// Make a plaintext login visibly fail for the client too.
					io.WriteString(conn, `<failure xmlns="`+nsSASL+`"><encryption-required/></failure>`)
					return
				}
				authed = true
				io.WriteString(conn, `<success xmlns="`+nsSASL+`"/>`)
			case "iq":
				var iq struct {
					ID   string    `xml:"id,attr"`
					Bind *struct{} `xml:"bind"`
				}
				if err := dec.DecodeElement(&iq, &el); err != nil {
					return
				}
				if iq.Bind != nil {
					fmt.Fprintf(conn, `<iq type="result" id="%s"><bind xmlns="%s"><jid>u@127.0.0.1/res</jid></bind></iq>`, iq.ID, nsBind)
				} else {
					fmt.Fprintf(conn, `<iq type="result" id="%s"/>`, iq.ID)
				}
			case "presence":
				var to string
				for _, a := range el.Attr {
					if a.Name.Local == "to" {
						to = a.Value
					}
				}
				dec.Skip()
				if !strings.Contains(to, "conference") {
					continue
				}
				m.update(rec, func(r *connRecord) { r.joined = true })
				fmt.Fprintf(conn, `<message xmlns="jabber:client" from="nwws@conference.nwws-oi.weather.gov/x" type="groupchat"><x xmlns="nwws-oi" cccc="KKCI" ttaaii="FTUS21" awipsid="TAFKORD" issue="2026-08-22T14:32:00Z" id="conn%d">TEXT %d</x></message>`, idx, idx)
				if idx == 0 {
					time.Sleep(200 * time.Millisecond)
					return // drop the connection
				}
			default:
				dec.Skip()
			}
		}
	}
}

func selfSignedCert(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}
