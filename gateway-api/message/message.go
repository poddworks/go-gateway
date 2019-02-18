package message

import (
	"net/http"
	"net/url"
)

const (
	MessageCtxString string = "Message"
)

type Message struct {
	// Method specifies the HTTP method (GET, POST, PUT, etc.).
	//
	Method string `json:"-,"`

	// For server requests Host specifies the host on which the URL
	// is sought. Per RFC 7230, section 5.4, this is either the value
	// of the "Host" header or the host name given in the URL itself.
	// It may be of the form "host:port". For international domain
	// names, Host may be in Punycode or Unicode form. Use
	// golang.org/x/net/idna to convert it to either format if
	// needed.
	//
	Host string `json:"-,omitempty"`

	// RemoteAddr allows HTTP servers and other software to record
	// the network address that sent the request.
	//
	RemoteAddr string `json:"-,omitempty"`

	// Header contains the request header fields either received
	// by the server.
	//
	Header http.Header `json:"-,"`

	// QueryParameters contains the request query arguments fields received
	// by the server.
	//
	QueryParameters url.Values `json:"-,"`

	// QueryParameters contains the request query arguments fields received
	// by the server.
	//
	PathParameters map[string]string `json:"-,"`

	// Body is the request's body.
	//
	Body []byte `json:"-,"`
}
