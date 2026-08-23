module github.com/jbuitt/nwws-go-client

go 1.27.0

require gosrc.io/xmpp v0.5.1

require (
	github.com/google/uuid v1.1.1 // indirect
	golang.org/x/xerrors v0.0.0-20190717185122-a985d3407aa7 // indirect
	nhooyr.io/websocket v1.6.5 // indirect
)

// Locally patched fork: see third_party/gosrc.io-xmpp/stanza/parser.go for
// what's changed and why (NextPacket is too strict about top-level stanza
// namespaces for real-world NWWS-OI MUC traffic).
replace gosrc.io/xmpp => ./third_party/gosrc.io-xmpp
