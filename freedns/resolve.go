package freedns

import (
	"strings"
	"time"

	"github.com/miekg/dns"
	"github.com/sirupsen/logrus"
	"github.com/xiangyu123/cosp_dns/whitedomain"
)

// spoofingProofResolver can resolve the DNS request with 100% confidence.
type spoofingProofResolver struct {
	fastUpstreamProvider   upstreamProvider
	cleanUpstreamProvider  upstreamProvider
	publicUpstreamProvider upstreamProvider
}

func newSpoofingProofResolver(fastUpstreamProvider upstreamProvider, cleanUpstreamProvider upstreamProvider, publicUpstreamProvider upstreamProvider) *spoofingProofResolver {
	return &spoofingProofResolver{
		fastUpstreamProvider:   fastUpstreamProvider,
		cleanUpstreamProvider:  cleanUpstreamProvider,
		publicUpstreamProvider: publicUpstreamProvider,
	}
}

// resovle returns the response and which upstream is used
func (resolver *spoofingProofResolver) resolve(q dns.Question, recursion bool, net string) (*dns.Msg, string) {
	type result struct {
		res *dns.Msg
		err error
	}

	fail := &dns.Msg{
		MsgHdr: dns.MsgHdr{
			Rcode: dns.RcodeServerFailure,
		},
	}

	fastCh := make(chan result, 4)
	cleanCh := make(chan result, 4)
	publicCh := make(chan result, 4)

	cleanUpstream := resolver.cleanUpstreamProvider.GetUpstream()
	fastUpstream := resolver.fastUpstreamProvider.GetUpstream()
	publicUpstream := resolver.publicUpstreamProvider.GetUpstream()

	var resChans []chan result
	var upstreams []string

	// 1. detected the upstream based on query type and domain, finally we get the upstream dns server slice with its result channel
	// 判断是否需要使用公网dns来解析或者直接转发到内网dns， 仅判断一次
	reqDomain := q.Name
	allDomains := whitedomain.GetAllDomains()

	// 判断请求的类型，如果是PTR，就往多个upstream发送解析请求，然后将结果合并
	// 如果是其他的类型，则先判断是否是白名单中的域名，如果是则转发请求到多个upstream,判断结果有没有数据，合并所有获取的结果
	// 如果不是白名单中的域名，则直接转发给公网的dns
	// 注意upstream和resChan的索引在每个对应的slice中需要一一对应
	switch q.Qtype {
	case dns.TypePTR:
		resChans = append(resChans, fastCh)
		resChans = append(resChans, cleanCh)
		upstreams = append(upstreams, fastUpstream)
		upstreams = append(upstreams, cleanUpstream)
	default:
		if containsDomain(reqDomain, allDomains) {
			resChans = append(resChans, fastCh)
			resChans = append(resChans, cleanCh)
			upstreams = append(upstreams, fastUpstream)
			upstreams = append(upstreams, cleanUpstream)
		} else {
			resChans = append(resChans, publicCh)
			upstreams = append(upstreams, publicUpstream)
		}
	}

	Q := func(ch chan result, upstream string) {
		logrus.WithFields(logrus.Fields{
			"function": "Q",
			"upstream": upstream,
			"chan":     ch,
		}).Info()
		res, err := naiveResolve(q, recursion, net, upstream)
		if res == nil {
			res = fail
		}
		ch <- result{res, err}
	}

	// 2. loop the upstream, try to resolve by the upstream server and merge the result
	for i, resChan := range resChans {
		if resChan != nil {
			log.Println("start ...")
			u := upstreams[i]
			go Q(resChan, u)
		}
	}

	// send timeout results
	go func() {
		time.Sleep(1900 * time.Millisecond)
		for _, resChan := range resChans {
			resChan <- result{fail, Error("timeout")}
		}
	}()

	// fan-out result with dns answer which length > 0. means that has dns resolv record.
	// think about it very carefully, give up fan-out
	// upstream order same as channel order, so we can retrived the upstream by index
	// if multi channel has data(all dns servers work), pick the first one.
	for index, resChan := range resChans {
		if resChan != nil {
			r := <-resChan
			// if r.res != nil && r.res.Rcode == dns.RcodeSuccess && containsRecord(r.res) {
			if r.res != nil && r.res.Rcode == dns.RcodeSuccess {
				ck := containsRecord(r.res)
				log.Println("ck is", ck)
				return r.res, upstreams[index]
			}
		}
	}

	failedUpstream := strings.Join(upstreams, ",")
	return fail, failedUpstream // return r.res, upstreams
}

func naiveResolve(q dns.Question, recursion bool, net string, upstream string) (*dns.Msg, error) {
	// send to multiple upstream server, and check if has data
	// wait all resovler's result, if both has nodata, just return ony, if one of resolver return data, return data
	// if has multi data, merge the answers to ony and return to client
	r := &dns.Msg{
		MsgHdr: dns.MsgHdr{
			Id:               dns.Id(),
			RecursionDesired: recursion,
		},
		Question: []dns.Question{q},
	}
	c := &dns.Client{Net: net}

	res, _, err := c.Exchange(r, upstream)
	if err != nil {
		log.WithFields(logrus.Fields{
			"op":       "start_resolve",
			"upstream": upstream,
			"domain":   q.Name,
			"res":      res,
		}).Error(err)
		// In case the Rcode is initialized as RcodeSuccess but the error occurs.
		// Without this, the wrong result may be cached and returned.
		if res != nil && res.Rcode == dns.RcodeSuccess {
			res = nil
		}
	}
	return res, err
}

// containsDomain check if the request domain in white domains
func containsDomain(reqHostRecord string, whiteDomains []string) bool {
	reqHostRecord = strings.TrimRight(reqHostRecord, ".")
	for _, domain := range whiteDomains {
		if strings.HasSuffix(reqHostRecord, domain) {
			return true
		}
	}
	return false
}

func containsRecord(res *dns.Msg) bool {
	var rrs []dns.RR
	q := res.Question[0]
	ck := dns.TypeToString[q.Qtype]
	log.Println("q.Qtype is", ck)

	if len(res.Answer) > 0 {
		for _, answerRR := range res.Answer {
			if answerRR.Header().Rrtype == q.Qtype && answerRR.Header().Class == q.Qclass && answerRR.Header().Name == q.Name {
				rrs = append(rrs, answerRR)
			}
		}
	}

	if len(rrs) > 0 {
		return true
	} else {
		return false
	}
}
