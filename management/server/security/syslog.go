package security

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

// SyslogConfig configures the syslog event sink.
type SyslogConfig struct {
	Network    string `json:"network"` // udp, tcp, tcp+tls
	Address    string `json:"address"`
	Facility   int    `json:"facility"` // syslog facility number, default 16 (local0)
	CAFile     string `json:"ca_file"`  // optional CA for tcp+tls
	ServerName string `json:"server_name"`
	Framing    string `json:"framing"` // newline (RFC6587 non-transparent) or octet
}

// SyslogSink forwards events as RFC 5424 lines over UDP/TCP/TLS. Emit never
// blocks: the queue is bounded at 1000 with drop-oldest and a once-a-minute
// drop counter log.
type SyslogSink struct {
	cfg     SyslogConfig
	q       chan Event
	mu      sync.Mutex
	dropped uint64
	stop    chan struct{}
	wg      sync.WaitGroup
	now     func() time.Time
	dialer  func() (net.Conn, error)
}

// NewSyslogSink builds and starts the sink.
func NewSyslogSink(cfg SyslogConfig) (*SyslogSink, error) {
	if cfg.Address == "" {
		return nil, fmt.Errorf("syslog address empty")
	}
	if cfg.Facility == 0 {
		cfg.Facility = 16 // local0
	}
	if cfg.Framing == "" {
		cfg.Framing = "newline"
	}
	s := &SyslogSink{
		cfg:  cfg,
		q:    make(chan Event, 1000),
		stop: make(chan struct{}),
		now:  time.Now,
	}
	s.dialer = s.dial
	s.wg.Add(1)
	go s.pump()
	return s, nil
}

// Emit queues an event; drops the oldest when full.
func (s *SyslogSink) Emit(ev Event) {
	select {
	case s.q <- ev:
	default:
		select {
		case <-s.q: // drop oldest
		default:
		}
		s.mu.Lock()
		s.dropped++
		s.mu.Unlock()
		select {
		case s.q <- ev:
		default:
		}
	}
}

// Close flushes pending events with a 2s budget and stops the pump.
func (s *SyslogSink) Close() {
	close(s.stop)
	done := make(chan struct{})
	go func() { s.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}
}

func (s *SyslogSink) dropLog() {
	s.mu.Lock()
	n := s.dropped
	s.dropped = 0
	s.mu.Unlock()
	if n > 0 {
		log.Warnf("security syslog sink dropped %d events (queue full)", n)
	}
}

func (s *SyslogSink) pump() {
	defer s.wg.Done()
	var conn net.Conn
	var backoff time.Duration
	dropTick := time.NewTicker(time.Minute)
	defer dropTick.Stop()
	for {
		select {
		case <-s.stop:
			for {
				select {
				case ev := <-s.q:
					if conn != nil {
						s.writeFrame(conn, ev)
					}
				default:
					s.dropLog()
					if conn != nil {
						conn.Close()
					}
					return
				}
			}
		case <-dropTick.C:
			s.dropLog()
		case ev := <-s.q:
			if conn == nil {
				c, err := s.dialer()
				if err != nil {
					if backoff == 0 {
						backoff = time.Second
					} else if backoff < 30*time.Second {
						backoff *= 2
					}
					select {
					case <-time.After(backoff):
					case <-s.stop:
						return
					}
					// requeue the event we consumed
					select {
					case s.q <- ev:
					default:
					}
					continue
				}
				backoff = 0
				conn = c
			}
			if err := s.writeFrame(conn, ev); err != nil {
				conn.Close()
				conn = nil
				select {
				case s.q <- ev: // try to keep the failed event
				default:
				}
			}
		}
	}
}

func marshalEvent(ev Event) ([]byte, error) { return json.Marshal(ev) }

func (s *SyslogSink) dial() (net.Conn, error) {
	d := &net.Dialer{Timeout: 5 * time.Second}
	switch s.cfg.Network {
	case "tcp":
		return d.Dial("tcp", s.cfg.Address)
	case "tcp+tls":
		tcfg := &tls.Config{ServerName: s.cfg.ServerName, MinVersion: tls.VersionTLS12}
		if s.cfg.CAFile != "" {
			pem, err := os.ReadFile(s.cfg.CAFile)
			if err != nil {
				return nil, fmt.Errorf("syslog ca: %w", err)
			}
			pool := x509.NewCertPool()
			pool.AppendCertsFromPEM(pem)
			tcfg.RootCAs = pool
		}
		return tls.DialWithDialer(d, "tcp", s.cfg.Address, tcfg)
	default: // udp
		return d.Dial("udp", s.cfg.Address)
	}
}

func syslogSeverity(sev Severity) int {
	switch sev {
	case SeverityHigh:
		return 2 // critical
	case SeverityMedium:
		return 4 // warning
	case SeverityLow:
		return 5 // notice
	}
	return 6 // informational
}

// writeFrame writes one RFC 5424 message in the configured framing.
func (s *SyslogSink) writeFrame(conn net.Conn, ev Event) error {
	body, _ := marshalEvent(ev)
	pri := s.cfg.Facility*8 + syslogSeverity(ev.Severity)
	hdr := fmt.Sprintf("<%d>1 %s - netbirdy %s - - ", pri,
		s.now().UTC().Format("2006-01-02T15:04:05Z"), ev.Type)
	payload := hdr + string(body)
	var frame []byte
	if s.cfg.Framing == "octet" && s.cfg.Network != "udp" {
		frame = []byte(fmt.Sprintf("%d %s", len(payload), payload))
	} else {
		frame = []byte(payload + "\n")
	}
	conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	_, err := conn.Write(frame)
	return err
}
