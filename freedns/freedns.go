package freedns

import (
	"io"
	"io/ioutil"
	"os"
	"strings"

	"github.com/jasonlvhit/gocron"
	"github.com/miekg/dns"
	"github.com/sirupsen/logrus"
	lumberjack "gopkg.in/natefinch/lumberjack.v2"
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

var (
	log           *logrus.Logger
	logFileOutput *lumberjack.Logger
	ss            chan uint = make(chan uint, 100000)
	ff            chan uint = make(chan uint, 100000)
	hostName      string
	lines         []string = make([]string, 0)
)

func init() {
	// 初始化日志
	log = logrus.New()
	log.SetFormatter(&logrus.JSONFormatter{})
	log.SetReportCaller(true)

	fileName := "/var/log/dns_proxy.log"
	logFileOutput = &lumberjack.Logger{
		Filename:   fileName,
		MaxSize:    500,
		MaxBackups: 7,
		MaxAge:     28,
		Compress:   true,
	}

	writers := []io.Writer{
		os.Stdout,
		logFileOutput,
	}

	fileAndStdoutWriter := io.MultiWriter(writers...)
	log.SetOutput(fileAndStdoutWriter)

	// 获取和设置主机名
	content, err := ioutil.ReadFile("/proc/sys/kernel/hostname")
	if err != nil {
		hostName = "host1"
	}

	hostName = strings.SplitN(string(content), ".", 2)[0]

	doc, err := ioutil.ReadFile("/etc/dns_proxy/blockip")
	if err != nil {
		panic(err)
	}

	tmpLines := strings.Split(string(doc), "\n")
	m := make(map[string]bool)
	for _, v := range tmpLines {
		vv := strings.TrimSpace(v)
		if _, ok := m[vv]; !ok {
			lines = append(lines, vv)
		}
	}
}

// Error is the freedns error type
type Error string

func (e Error) Error() string {
	return string(e)
}

func executeCronJob() {
	log.WithFields(logrus.Fields{
		"op":   "start_cron",
		"task": "collectMetrics()",
	}).Info()

	gocron.Every(1).Minute().Do(collectMetrics)
	// gocron.Every(1).Second().Do(collectMetrics)
	<-gocron.Start()
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

	// 运行定时任务
	go executeCronJob()

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

func contains(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

func (s *Server) handle(w dns.ResponseWriter, req *dns.Msg, net string) {
	res := &dns.Msg{}
	var upstream string

	if len(req.Question) < 1 {
		res.SetRcode(req, dns.RcodeBadName)
		w.WriteMsg(res)
		log.WithFields(logrus.Fields{
			"op":  "handle",
			"msg": "request without questions",
		}).Warn()
		return
	}

	// black list
	clientAddrPort := w.RemoteAddr().String()
	clientAddr := strings.Split(clientAddrPort, ":")[0]

	if contains(lines, clientAddr) {
		logger := log.WithFields(logrus.Fields{
			"client": clientAddr,
			"op":     "block",
		})
		logger.Warn()
		w.WriteMsg(&dns.Msg{})
	} else {
		// 正式开始解析
		res, upstream = s.lookup(req, net)
		w.WriteMsg(res)

		// logging
		l := log.WithFields(logrus.Fields{
			"op":       "handle",
			"domain":   req.Question[0].Name,
			"type":     dns.TypeToString[req.Question[0].Qtype],
			"upstream": upstream,
			"status":   dns.RcodeToString[res.Rcode],
			"client":   clientAddr,
			"rcode":    res.Rcode,
		})

		if res.Rcode == dns.RcodeServerFailure {
			l.Warn()
			ff <- 1
		} else {
			l.Info()
			ss <- 1
		}
	}
}

// lookup queries the dns request `q` on all of the resolvers,
// and returns the result and which upstream is used.
func (s *Server) lookup(req *dns.Msg, net string) (*dns.Msg, string) {
	// dns.Msg.SetReply() always set the Rcode to RcodeSuccess  which we do not want
	res, upstream := s.resolver.resolve(req.Question[0], req.RecursionDesired, net)
	if res.Rcode == dns.RcodeSuccess || res.Rcode == dns.RcodeNameError {
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
