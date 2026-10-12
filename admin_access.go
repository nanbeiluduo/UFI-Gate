package main

import (
	"net"
	"net/http"
)

// Refuse unauthorized connections before the HTTP server reads a request.
// Existing iptables DROP rules remain the first layer while network enforcement is enabled.
type adminListener struct {
	net.Listener
	gate *Gate
}

func (g *Gate) adminAddressAllowed(address string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.ctx.Err() != nil {
		return false
	}
	if g.db.Config.AdminHash == "" {
		return true
	}
	mac, err := g.peerMAC(&http.Request{RemoteAddr: address})
	return err == nil && g.isAdminDevice(mac)
}

func (l *adminListener) Accept() (net.Conn, error) {
	for {
		conn, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		if l.gate.adminAddressAllowed(conn.RemoteAddr().String()) {
			return conn, nil
		}
		_ = conn.Close()
	}
}

func (g *Gate) protectAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Recheck every request, including keep-alive connections opened before revocation/setup.
		if !g.adminAddressAllowed(r.RemoteAddr) {
			panic(http.ErrAbortHandler) // net/http closes the connection without sending an HTTP response.
		}
		next.ServeHTTP(w, r)
	})
}
