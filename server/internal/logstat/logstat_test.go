package logstat

import (
	"testing"
	"time"
)

func TestFeed(t *testing.T) {
	now := time.Date(2026, 11, 19, 21, 0, 0, 0, time.Local)
	s := &Stats{Now: func() time.Time { return now }}
	s.Load()
	s.saved = time.Now()
	lines := []string{
		`{"type":"info","payload":"[TCP] 192.168.1.5:5000 --> www.google.com:443 match DomainSuffix(google.com) using Proxy[JP]"}`,
		`{"type":"info","payload":"[TCP] 192.168.1.5:5001 --> ads.x.com:443 match AND((RuleSet(prism-adblock)),(NOT((RuleSet(prism-adallow))))) using REJECT"}`,
		`{"type":"info","payload":"[UDP] 192.168.1.5:5002 --> 198.18.0.3:443 match RuleSet(prism-adblock) using REJECT"}`,
		`{"type":"info","payload":"[TCP] 192.168.1.5:5003 --> evil.com:443 match DomainKeyword(evil) using REJECT"}`,
		`{"type":"debug","payload":"[DNS] cache hit for www.google.com., expire at 2026"}`,
		`{"type":"debug","payload":"[DNS] www.baidu.com --> [1.2.3.4] A from udp://223.5.5.5:53"}`,
	}
	for _, l := range lines {
		s.Feed([]byte(l))
	}
	s.DNSSample(100)
	r := s.Report()
	if r.Req24 != 4 || r.Blk24 != 2 || r.Today != 2 {
		t.Fatalf("%+v", r)
	}
	if len(r.Top) != 1 || r.Top[0].Domain != "ads.x.com" {
		t.Fatalf("%+v", r.Top)
	}
	if r.DNSResult.Cache != 1 || r.DNSResult.Upstream != 1 || r.DNSResult.Total != 2 {
		t.Fatalf("%+v", r.DNSResult)
	}
	if r.DNS.Avg == nil || *r.DNS.Avg != 100 {
		t.Fatal("dns avg")
	}
}
