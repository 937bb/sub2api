package service

import (
	"context"
	"errors"
	"net"
	"strconv"
	"strings"
	"time"
)

// SSRF 防护 helper：
//   - validateEndpoint 在 admin 提交时阻止 http/loopback/私网/云元数据 URL
//   - safeDialContext 在 socket 层再次校验真实 IP，防止 DNS rebinding
//
// 已知 cloud metadata hostname 拒绝列表（小写比较）。
var monitorBlockedHostnames = map[string]struct{}{
	"localhost":                  {},
	"localhost.localdomain":      {},
	"metadata":                   {},
	"metadata.google.internal":   {},
	"metadata.goog":              {},
	"instance-data":              {},
	"instance-data.ec2.internal": {},
}

// CIDR 列表：包含所有需要拒绝的 IPv4/IPv6 段。
// 解析时只 panic 一次（启动时确认），生产路径只做 Contains。
var monitorBlockedCIDRs = mustParseCIDRs([]string{
	"0.0.0.0/8",       // current network
	"10.0.0.0/8",      // RFC1918
	"100.64.0.0/10",   // shared address space
	"127.0.0.0/8",     // loopback
	"169.254.0.0/16",  // link-local and cloud metadata
	"172.16.0.0/12",   // RFC1918
	"192.0.0.0/24",    // IETF protocol assignments; global anycast exceptions below
	"192.0.2.0/24",    // documentation
	"192.88.99.0/24",  // deprecated 6to4 relay anycast
	"192.168.0.0/16",  // RFC1918
	"198.18.0.0/15",   // benchmarking
	"198.51.100.0/24", // documentation
	"203.0.113.0/24",  // documentation
	"224.0.0.0/4",     // multicast
	"240.0.0.0/4",     // reserved and limited broadcast
	"::/128",          // unspecified
	"::1/128",         // loopback
	"64:ff9b:1::/48",  // local-use IPv4/IPv6 translation
	"100::/64",        // discard-only
	"100:0:0:1::/64",  // dummy IPv6 prefix
	"2001::/23",       // IETF protocol assignments; global exceptions below
	"2001:db8::/32",   // documentation
	"2002::/16",       // deprecated 6to4
	"3fff::/20",       // documentation
	"5f00::/16",       // segment-routing SIDs
	"fc00::/7",        // unique local
	"fe80::/10",       // link-local
	"ff00::/8",        // multicast
})

// These are the globally reachable assignments nested in otherwise
// non-global IANA special-purpose parent blocks above.
var monitorGlobalCIDRExceptions = mustParseCIDRs([]string{
	"192.0.0.9/32",    // Port Control Protocol anycast
	"192.0.0.10/32",   // Traversal Using Relays around NAT anycast
	"2001:1::1/128",   // Port Control Protocol anycast
	"2001:1::2/128",   // Traversal Using Relays around NAT anycast
	"2001:1::3/128",   // DNS-SD service registration protocol anycast
	"2001:3::/32",     // Automatic Multicast Tunneling
	"2001:4:112::/48", // AS112 direct delegation
	"2001:20::/28",    // ORCHIDv2
	"2001:30::/28",    // Drone Remote ID Protocol Entity Tags
})

var errMonitorDialPolicy = errors.New("channel monitor dial blocked by policy")

// monitorDialer 共享 Dialer，与 net/http 默认值对齐。
var monitorDialer = &net.Dialer{
	Timeout:   monitorDialTimeout,
	KeepAlive: monitorDialKeepAlive,
}

var monitorDialContext = monitorDialer.DialContext
var monitorLookupIPAddr = net.DefaultResolver.LookupIPAddr
var monitorDialFallbackDelay = 250 * time.Millisecond

// mustParseCIDRs 在包初始化时解析 CIDR 字符串，失败 panic。
func mustParseCIDRs(cidrs []string) []*net.IPNet {
	out := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			panic("channel_monitor_ssrf: invalid CIDR " + c + ": " + err.Error())
		}
		out = append(out, n)
	}
	return out
}

// isBlockedHostname 判断 hostname 是否命中黑名单。
func isBlockedHostname(hostname string) bool {
	if hostname == "" {
		return true
	}
	_, blocked := monitorBlockedHostnames[strings.ToLower(hostname)]
	return blocked
}

// isPrivateIP 判断 IP 是否落在禁止段（loopback/RFC1918/link-local/ULA 等）。
func isPrivateIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	for _, n := range monitorGlobalCIDRExceptions {
		if n.Contains(ip) {
			return false
		}
	}
	if !ip.IsGlobalUnicast() || ip.IsUnspecified() || ip.IsLoopback() || ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() || ip.IsMulticast() {
		return true
	}
	for _, n := range monitorBlockedCIDRs {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// isPrivateOrLoopbackHost 解析 hostname 的所有 A/AAAA 记录，
// 任一 IP 落在私网/loopback 段即认为不安全。
//
// hostname 是 IP 字面量时也走同一路径。
func isPrivateOrLoopbackHost(ctx context.Context, hostname string) (bool, error) {
	if isBlockedHostname(hostname) {
		return true, nil
	}
	// IP 字面量直接判断。
	if ip := net.ParseIP(hostname); ip != nil {
		return isPrivateIP(ip), nil
	}
	addrs, err := monitorLookupIPAddr(ctx, hostname)
	if err != nil {
		return false, err
	}
	if len(addrs) == 0 {
		return true, nil
	}
	for _, a := range addrs {
		if isPrivateIP(a.IP) {
			return true, nil
		}
	}
	return false, nil
}

// safeDialContext 在真实 dial 前再次校验目标 IP，防止 DNS rebinding。
// 解析 hostname 后逐个 IP 尝试连接，命中私网即拒绝（即便 validateEndpoint 时返回的是公网 IP）。
func safeDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, errMonitorDialPolicy
	}
	if host == "" || strings.Contains(host, "%") || !validMonitorPort(port) {
		return nil, errMonitorDialPolicy
	}
	// 字面量 IP 走快速路径。
	if ip := net.ParseIP(host); ip != nil {
		if isPrivateIP(ip) {
			return nil, errMonitorDialPolicy
		}
		return monitorDialContext(ctx, network, address)
	}
	if isBlockedHostname(host) {
		return nil, errMonitorDialPolicy
	}
	addrs, err := monitorLookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(addrs) == 0 {
		return nil, errMonitorDialPolicy
	}
	eligible := make([]net.IPAddr, 0, len(addrs))
	for _, a := range addrs {
		if isPrivateIP(a.IP) {
			return nil, errMonitorDialPolicy
		}
		eligible = append(eligible, a)
	}
	if len(eligible) == 0 {
		return nil, errMonitorDialPolicy
	}
	return dialMonitorAddresses(ctx, network, port, eligible)
}

type monitorDialResult struct {
	id   int
	conn net.Conn
	err  error
}

func dialMonitorAddresses(ctx context.Context, network, port string, addrs []net.IPAddr) (net.Conn, error) {
	if len(addrs) == 0 {
		return nil, errMonitorDialPolicy
	}

	dialCtx, cancel := context.WithTimeout(ctx, monitorDialTimeout)
	defer cancel()

	const maxActive = 2
	type attempt struct {
		cancel   context.CancelFunc
		retiring bool
	}
	results := make(chan monitorDialResult, maxActive)
	attempts := make(map[int]*attempt, maxActive)
	order := make([]int, 0, maxActive)
	next, nextID := 0, 0
	start := func() {
		addr := net.JoinHostPort(addrs[next].IP.String(), port)
		next++
		id := nextID
		nextID++
		attemptCtx, attemptCancel := context.WithCancel(dialCtx)
		attempts[id] = &attempt{cancel: attemptCancel}
		order = append(order, id)
		go func() {
			conn, err := monitorDialContext(attemptCtx, network, addr)
			results <- monitorDialResult{id: id, conn: conn, err: err}
		}()
	}
	cancelAttempts := func() {
		for _, attempt := range attempts {
			attempt.cancel()
		}
	}
	drainAttempts := func(winner net.Conn) {
		for len(attempts) > 0 {
			result := <-results
			attempts[result.id].cancel()
			delete(attempts, result.id)
			if result.conn != nil && result.conn != winner {
				_ = result.conn.Close()
			}
		}
	}

	start()
	timer := time.NewTimer(monitorDialFallbackDelay)
	defer timer.Stop()
	var lastErr error
	for len(attempts) > 0 {
		select {
		case <-dialCtx.Done():
			cancelAttempts()
			drainAttempts(nil)
			return nil, dialCtx.Err()
		case result := <-results:
			finishedAttempt := attempts[result.id]
			finishedAttempt.cancel()
			delete(attempts, result.id)
			for i, id := range order {
				if id == result.id {
					order = append(order[:i], order[i+1:]...)
					break
				}
			}
			if result.err == nil && !finishedAttempt.retiring {
				cancelAttempts()
				drainAttempts(result.conn)
				return result.conn, nil
			}
			if result.conn != nil {
				_ = result.conn.Close()
			}
			if result.err != nil {
				lastErr = result.err
			}
			if next < len(addrs) && len(attempts) < maxActive {
				start()
			}
		case <-timer.C:
			if next < len(addrs) && len(attempts) < maxActive {
				start()
			} else if next < len(addrs) {
				for _, id := range order {
					if attempt := attempts[id]; !attempt.retiring {
						attempt.retiring = true
						attempt.cancel()
						break
					}
				}
			}
			if next < len(addrs) || len(attempts) > 0 {
				timer.Reset(monitorDialFallbackDelay)
			}
		}
	}
	return nil, lastErr
}

func validMonitorPort(port string) bool {
	n, err := strconv.ParseUint(port, 10, 16)
	return err == nil && n != 0
}
