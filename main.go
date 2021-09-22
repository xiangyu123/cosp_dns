package main

import (
	"flag"
	"log"
	"os"

	_ "net/http/pprof"

	"github.com/xiangyu123/cosp_dns/freedns"
)

func main() {
	/*
		go func() {
			log.Println(http.ListenAndServe("localhost:6060", nil))
		}()
	*/

	var (
		fastUpstream   string
		cleanUpstream  string
		publicUpstream string
		listen         string
		logLevel       string
		// cache         bool
	)

	flag.StringVar(&fastUpstream, "f", "114.114.114.114:53", "The first-local recursion DNS upstream, ip:port")
	flag.StringVar(&cleanUpstream, "c", "8.8.8.8:53", "The second-local recursion DNS upstream., ip:port")
	flag.StringVar(&publicUpstream, "p", "8.8.8.8:53", "The public-remote recursion DNS upstream., ip:port")
	flag.StringVar(&listen, "l", "0.0.0.0:53", "Listening address.")
	// flag.BoolVar(&cache, "cache", true, "Enable cache.")
	flag.StringVar(&logLevel, "log-level", "", "Set log level: info/warn/error.")

	flag.Parse()

	s, err := freedns.NewServer(freedns.Config{
		FastUpstream:   fastUpstream,
		CleanUpstream:  cleanUpstream,
		PublicUpstream: publicUpstream,
		Listen:         listen,
		LogLevel:       logLevel,
	})
	if err != nil {
		log.Fatalln(err)
		os.Exit(-1)
	}

	log.Fatalln(s.Run())
	os.Exit(-1)
}
