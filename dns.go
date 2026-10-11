package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

// Bounded RFC 1035 codec: only one question, standard IN queries, no arbitrary compressed input names.
func question(b []byte) (string, uint16, int, error) {
	if len(b) < 17 || len(b) > 4096 || binary.BigEndian.Uint16(b[4:6]) != 1 || b[2]&0x80 != 0 || b[2]&0x78 != 0 {
		return "", 0, 0, fmt.Errorf("invalid DNS query")
	}
	pos := 12
	labels := []string{}
	total := 0
	for {
		if pos >= len(b) {
			return "", 0, 0, io.ErrUnexpectedEOF
		}
		n := int(b[pos])
		pos++
		if n == 0 {
			break
		}
		if n > 63 || pos+n > len(b) {
			return "", 0, 0, fmt.Errorf("invalid DNS label")
		}
		label := string(b[pos : pos+n])
		if strings.ContainsAny(label, "\x00\r\n") {
			return "", 0, 0, fmt.Errorf("invalid DNS name")
		}
		labels = append(labels, label)
		pos += n
		total += n + 1
		if total > 254 {
			return "", 0, 0, fmt.Errorf("DNS name too long")
		}
	}
	if pos+4 > len(b) || binary.BigEndian.Uint16(b[pos+2:pos+4]) != 1 {
		return "", 0, 0, fmt.Errorf("only IN supported")
	}
	return strings.ToLower(strings.Join(labels, ".")), binary.BigEndian.Uint16(b[pos : pos+2]), pos + 4, nil
}
func matches(name string, list []string) bool {
	name = strings.TrimSuffix(strings.ToLower(name), ".")
	for _, rule := range list {
		rule = strings.ToLower(strings.TrimSpace(rule))
		if strings.HasPrefix(rule, "*.") {
			root := rule[2:]
			if name == root || strings.HasSuffix(name, "."+root) {
				return true
			}
		} else if name == rule {
			return true
		}
	}
	return false
}
func dnsAnswer(q []byte, end int, typ uint16, ip string) []byte {
	a := append([]byte{}, q[:end]...)
	a[2] = 0x81
	a[3] = 0x80
	a[6] = 0
	a[7] = 0
	for i := 8; i < 12; i++ {
		a[i] = 0
	}
	if typ == 1 && net.ParseIP(ip).To4() != nil {
		a[7] = 1
		a = append(a, 0xc0, 0x0c, 0, 1, 0, 1, 0, 0, 0, 0, 0, 4)
		a = append(a, net.ParseIP(ip).To4()...)
	}
	return a
}
func skipDNSName(b []byte, p int) (int, error) {
	for steps := 0; steps < 128; steps++ {
		if p >= len(b) {
			return 0, io.ErrUnexpectedEOF
		}
		n := int(b[p])
		p++
		if n == 0 {
			return p, nil
		}
		if n&0xc0 == 0xc0 {
			if p >= len(b) {
				return 0, io.ErrUnexpectedEOF
			}
			return p + 1, nil
		}
		if n > 63 || p+n > len(b) {
			return 0, fmt.Errorf("bad name")
		}
		p += n
	}
	return 0, fmt.Errorf("name too long")
}
func answerIPs(b []byte) (map[string]int64, error) {
	if len(b) < 12 {
		return nil, io.ErrUnexpectedEOF
	}
	p := 12
	questions := int(binary.BigEndian.Uint16(b[4:6]))
	answers := int(binary.BigEndian.Uint16(b[6:8]))
	if questions != 1 || answers > 128 {
		return nil, fmt.Errorf("bad reply")
	}
	var e error
	p, e = skipDNSName(b, p)
	if e != nil || p+4 > len(b) {
		return nil, fmt.Errorf("bad question")
	}
	p += 4
	ips := map[string]int64{}
	for i := 0; i < answers; i++ {
		p, e = skipDNSName(b, p)
		if e != nil || p+10 > len(b) {
			return nil, fmt.Errorf("bad answer")
		}
		typ := binary.BigEndian.Uint16(b[p : p+2])
		class := binary.BigEndian.Uint16(b[p+2 : p+4])
		ttl := binary.BigEndian.Uint32(b[p+4 : p+8])
		n := int(binary.BigEndian.Uint16(b[p+8 : p+10]))
		p += 10
		if p+n > len(b) {
			return nil, io.ErrUnexpectedEOF
		}
		if typ == 1 && class == 1 && n == 4 {
			if ttl < 10 {
				ttl = 10
			}
			if ttl > 300 {
				ttl = 300
			}
			ips[net.IP(b[p:p+4]).String()] = time.Now().Unix() + int64(ttl)
		}
		p += n
	}
	return ips, nil
}
func upstreamDNS(q []byte) ([]byte, error) {
	for _, server := range []string{"223.5.5.5:53", "119.29.29.29:53"} {
		c, e := net.DialTimeout("udp4", server, 2*time.Second)
		if e != nil {
			continue
		}
		c.SetDeadline(time.Now().Add(2 * time.Second))
		_, e = c.Write(q)
		buf := make([]byte, 4096)
		n := 0
		if e == nil {
			n, e = c.Read(buf)
		}
		c.Close()
		if e != nil || n < 12 || buf[0] != q[0] || buf[1] != q[1] || buf[2]&0x80 == 0 {
			continue
		}
		if buf[2]&2 != 0 {
			tc, e := net.DialTimeout("tcp4", server, 2*time.Second)
			if e != nil {
				continue
			}
			tc.SetDeadline(time.Now().Add(3 * time.Second))
			frame := make([]byte, 2)
			binary.BigEndian.PutUint16(frame, uint16(len(q)))
			_, e = tc.Write(append(frame, q...))
			if e == nil {
				_, e = io.ReadFull(tc, frame)
			}
			if e == nil {
				size := int(binary.BigEndian.Uint16(frame))
				if size < 12 || size > 16384 {
					e = fmt.Errorf("DNS reply too large")
				} else {
					buf = make([]byte, size)
					_, e = io.ReadFull(tc, buf)
					n = size
				}
			}
			tc.Close()
			if e != nil || buf[0] != q[0] || buf[1] != q[1] {
				continue
			}
		}
		return buf[:n], nil
	}
	return nil, fmt.Errorf("DNS upstream unavailable")
}
func (g *Gate) dnsReply(q []byte, ip string) ([]byte, error) {
	name, typ, end, e := question(q)
	if e != nil {
		return nil, e
	}
	g.mu.Lock()
	c := g.db.Config
	d := g.dnsDevice(ip)
	allowed := d != nil && g.allowed(d)
	black := matches(name, c.Black)
	white := matches(name, c.White)
	blocked := d != nil && (d.Banned || g.state(d) == "quota")
	if black && d != nil {
		g.blackPackets++
		key := "black:" + d.MAC + ":" + name
		if g.attempt(key) {
			g.event("blacklist", d.MAC+" 请求 "+name)
			g.attempts[key] = Attempt{Until: time.Now().Add(time.Minute).Unix()}
		}
	}
	g.mu.Unlock()
	if c.PortalHost != "" && name == strings.ToLower(c.PortalHost) {
		return dnsAnswer(q, end, typ, c.Gateway), nil
	}
	if d == nil {
		// Never expose a recursive DNS service to non-hotspot callers. Newly joined clients are discovered within the next 5-second sample.
		if c.Enabled {
			return dnsAnswer(q, end, typ, c.Gateway), nil
		}
		return dnsAnswer(q, end, typ, ""), nil
	}
	if !c.Enabled {
		return upstreamDNS(q)
	}
	if black || blocked || (!allowed && !white) {
		if black && !demo {
			g.learnBlack(name)
		}
		return dnsAnswer(q, end, typ, c.Gateway), nil
	}
	// IPv6 is deliberately disabled across the hotspot. Return NODATA for AAAA to avoid unusable IPv6 answers.
	if typ == 28 {
		return dnsAnswer(q, end, typ, ""), nil
	}
	answer, e := upstreamDNS(q)
	if e != nil {
		return nil, e
	}
	if white {
		ips, e := answerIPs(answer)
		if e != nil {
			return nil, e
		}
		g.mu.Lock()
		changed := false
		for ip, expiry := range ips {
			if _, exists := g.resolvedWhite[ip]; !exists {
				changed = true
			}
			g.resolvedWhite[ip] = expiry
		}
		if changed && g.db.Config.Enabled {
			if e = g.reconcile(); e != nil {
				g.mu.Unlock()
				return nil, e
			}
		}
		g.mu.Unlock()
	}
	return answer, nil
}

// Resolve blacklisted names out of band to block cached/active IPv4 flows as well as DNS. Encrypted DNS and shared CDN IPs remain documented limits.
func (g *Gate) learnBlack(name string) {
	select {
	case g.dnsSlots <- struct{}{}:
	default:
		return
	}
	go func() {
		defer func() { <-g.dnsSlots }()
		q := make([]byte, 12)
		q[0] = byte(time.Now().UnixNano())
		q[1] = byte(time.Now().UnixNano() >> 8)
		q[2] = 1
		q[5] = 1
		for _, label := range strings.Split(name, ".") {
			q = append(q, byte(len(label)))
			q = append(q, []byte(label)...)
		}
		q = append(q, 0, 0, 1, 0, 1)
		a, e := upstreamDNS(q)
		if e != nil {
			return
		}
		ips, e := answerIPs(a)
		if e != nil {
			return
		}
		g.mu.Lock()
		defer g.mu.Unlock()
		if !g.db.Config.Enabled || !matches(name, g.db.Config.Black) {
			return
		}
		changed := false
		for ip, expiry := range ips {
			if _, ok := g.resolvedBlack[ip]; !ok {
				changed = true
			}
			g.resolvedBlack[ip] = expiry
		}
		if changed {
			g.reconcile()
		}
	}()
}
func (g *Gate) serveDNS(udp net.PacketConn, tcp net.Listener) {
	go func() {
		for {
			c, e := tcp.Accept()
			if e != nil {
				return
			}
			select {
			case g.dnsSlots <- struct{}{}:
				go func(c net.Conn) {
					defer func() { <-g.dnsSlots; c.Close() }()
					c.SetDeadline(time.Now().Add(6 * time.Second))
					h := make([]byte, 2)
					if _, e := io.ReadFull(c, h); e != nil {
						return
					}
					n := int(binary.BigEndian.Uint16(h))
					if n < 17 || n > 4096 {
						return
					}
					q := make([]byte, n)
					if _, e := io.ReadFull(c, q); e != nil {
						return
					}
					ip, _, _ := net.SplitHostPort(c.RemoteAddr().String())
					a, e := g.dnsReply(q, ip)
					if e != nil {
						return
					}
					binary.BigEndian.PutUint16(h, uint16(len(a)))
					c.Write(append(h, a...))
				}(c)
			default:
				c.Close()
			}
		}
	}()
	buf := make([]byte, 4096)
	for {
		n, addr, e := udp.ReadFrom(buf)
		if e != nil {
			return
		}
		q := append([]byte{}, buf[:n]...)
		select {
		case g.dnsSlots <- struct{}{}:
			go func() {
				defer func() { <-g.dnsSlots }()
				ip, _, _ := net.SplitHostPort(addr.String())
				a, e := g.dnsReply(q, ip)
				if e == nil {
					udp.WriteTo(a, addr)
				}
			}()
		default:
		}
	}
}
func captchaSVG(answer string) string {
	var b strings.Builder
	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" width="220" height="72" viewBox="0 0 220 72"><rect width="220" height="72" rx="8" fill="#eef3fb"/>`)
	noise := make([]byte, 40)
	for i := range noise {
		noise[i] = byte(i*37 + int(answer[i%len(answer)]))
	}
	for i := 0; i < 16; i++ {
		fmt.Fprintf(&b, `<path d="M%d %d L%d %d" stroke="#b2c0d3" opacity=".7"/>`, int(noise[i])*220/256, int(noise[i+1])*72/256, int(noise[i+2])*220/256, int(noise[i+3])*72/256)
	}
	for i, c := range answer {
		fmt.Fprintf(&b, `<text x="%d" y="%d" font-family="sans-serif" font-weight="700" font-size="30" fill="#22456f" transform="rotate(%d %d 40)">%c</text>`, 20+i*39, 45+int(noise[i])%9, int(noise[i])%25-12, 20+i*39, c)
	}
	b.WriteString(`</svg>`)
	return b.String()
}
