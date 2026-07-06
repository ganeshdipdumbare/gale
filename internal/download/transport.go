package download

import (
	"net"
	"net/http"
	"time"
)

// sharedTransport is tuned for parallel range downloads to the same host.
var sharedTransport = &http.Transport{
	Proxy: http.ProxyFromEnvironment,
	DialContext: (&net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext,
	MaxIdleConns:          100,
	MaxIdleConnsPerHost:   32,
	MaxConnsPerHost:       32,
	IdleConnTimeout:       90 * time.Second,
	TLSHandshakeTimeout:   10 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
	ForceAttemptHTTP2:     true,
}

// NewDownloader returns a downloader with sensible defaults.
func NewDownloader() *Downloader {
	return &Downloader{
		Client: &http.Client{
			Timeout:   0,
			Transport: sharedTransport,
		},
	}
}
