package freedns

import (
	"github.com/miekg/dns"
	"github.com/sirupsen/logrus"
)

// Config stores the configuration for the Server
type Config struct {
	FastUpstream   string
	CleanUpstream  string
	PublicUpstream string
	Listen         string
	LogLevel       string
}

// Server is type of the freedns server instance
type Server struct {
	config Config

	udpServer *dns.Server
	tcpServer *dns.Server

	resolver *spoofingProofResolver
}

var log = logrus.New()

// Error is the freedns error type
type Error string

func (e Error) Error() string {
	return string(e)
}

// NewServer creates a new freedns server instance.
func NewServer(cfg Config) (*Server, error) {
	s := &Server{}

	// set log level
	if level, parseError := logrus.ParseLevel(cfg.LogLevel); parseError == nil {
		log.SetLevel(level)
	}

	// normalize and setlisten address
	if cfg.Listen == "" {
		cfg.Listen = "0.0.0.0"
	}
	var err error
	if cfg.Listen, err = normalizeDnsAddress(cfg.Listen); err != nil {
		return nil, err
	}

	s.config = cfg

	var fastUpstreamProvider, cleanUpstreamProvider, publicUpstreamProvider upstreamProvider
	fastUpstreamProvider, err = newUpstreamProvider(cfg.FastUpstream)
	if err != nil {
		return nil, err
	}
	cleanUpstreamProvider, err = newUpstreamProvider(cfg.CleanUpstream)
	if err != nil {
		return nil, err
	}
	publicUpstreamProvider, err = newUpstreamProvider(cfg.PublicUpstream)
	if err != nil {
		return nil, err
	}

	s.config = cfg
	s.udpServer = &dns.Server{
		Addr: s.config.Listen,
		Net:  "udp",
		Handler: dns.HandlerFunc(func(w dns.ResponseWriter, req *dns.Msg) {
			s.handle(w, req, "udp")
		}),
	}

	s.tcpServer = &dns.Server{
		Addr: s.config.Listen,
		Net:  "tcp",
		Handler: dns.HandlerFunc(func(w dns.ResponseWriter, req *dns.Msg) {
			s.handle(w, req, "tcp")
		}),
	}

	s.resolver = newSpoofingProofResolver(fastUpstreamProvider, cleanUpstreamProvider, publicUpstreamProvider)

	return s, nil
}

// Run tcp and udp server.
func (s *Server) Run() error {
	errChan := make(chan error, 2)

	go func() {
		err := s.tcpServer.ListenAndServe()
		errChan <- err
	}()

	go func() {
		err := s.udpServer.ListenAndServe()
		errChan <- err
	}()

	select {
	case err := <-errChan:
		s.tcpServer.Shutdown()
		s.udpServer.Shutdown()
		return err
	}
}

// Shutdown shuts down the freedns server
func (s *Server) Shutdown() {
	s.tcpServer.Shutdown()
	s.udpServer.Shutdown()
}

func (s *Server) handle(w dns.ResponseWriter, req *dns.Msg, net string) {
	res := &dns.Msg{}

	if len(req.Question) < 1 {
		res.SetRcode(req, dns.RcodeBadName)
		w.WriteMsg(res)
		log.WithFields(logrus.Fields{
			"op":  "handle",
			"msg": "request without questions",
		}).Warn()
		return
	}

	res, upstream := s.lookup(req, net)
	w.WriteMsg(res)

	// logging
	l := log.WithFields(logrus.Fields{
		"op":       "handle",
		"domain":   req.Question[0].Name,
		"type":     dns.TypeToString[req.Question[0].Qtype],
		"upstream": upstream,
		"status":   dns.RcodeToString[res.Rcode],
	})
	if res.Rcode == dns.RcodeSuccess {
		l.Info()
	} else {
		l.Warn()
	}
}

// lookup queries the dns request `q` on all of the resolvers,
// and returns the result and which upstream is used.
func (s *Server) lookup(req *dns.Msg, net string) (*dns.Msg, string) {
	log.Println("start to debug.....")
	// dns.Msg.SetReply() always set the Rcode to RcodeSuccess  which we do not want
	res, upstream := s.resolver.resolve(req.Question[0], req.RecursionDesired, net)

	log.Println("res.Rcode is", res.Rcode)

	if res.Rcode == dns.RcodeSuccess {
		log.WithFields(logrus.Fields{
			"op":       "resolve_success",
			"domain":   req.Question[0].Name,
			"type":     dns.TypeToString[req.Question[0].Qtype],
			"upstream": upstream,
		}).Info()
	}

	rcode := res.Rcode
	res.SetReply(req)
	res.Rcode = rcode
	return res, upstream
}
