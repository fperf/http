package http

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/fperf/fperf"
)

type options struct {
	keepalive bool
	urls      []string
	method    string
	userAgent string
	body      string
	lb        string
	timeout   time.Duration
}
type httpClient struct {
	cli  http.Client
	opts options
	lb   func() int
}

func newHTTPClient(flag *fperf.FlagSet) fperf.Client {
	c := new(httpClient)
	flag.BoolVar(&c.opts.keepalive, "keepalive", true, "keep connection alive")
	flag.StringVar(&c.opts.method, "method", "GET", "method of HTTP request, methods:GET,POST,HEAD,OPTIONS,PUT,DELETE")
	flag.StringVar(&c.opts.userAgent, "user-agent", "fperf-http-client", "customize the header User-Agent")
	flag.StringVar(&c.opts.body, "body", "", "content of request body")
	flag.StringVar(&c.opts.lb, "lb", "rr", "load banlancer, can be none, rr or rand")
	flag.DurationVar(&c.opts.timeout, "timeout", 10*time.Second, "timeout of request")
	flag.Usage = func() {
		fmt.Printf("Usage: http [options] <url>\noptions:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if len(flag.Args()) == 0 {
		flag.Usage()
		os.Exit(-1)
	}

	c.opts.urls = strings.Split(flag.Arg(0), ";")
	if len(c.opts.urls) == 1 {
		c.lb = loadBalancer("none", 0)
	} else {
		c.lb = loadBalancer(c.opts.lb, len(c.opts.urls))
	}
	return c
}

func (c *httpClient) Dial(addr string) error {
	tr := &http.Transport{
		DisableKeepAlives: !c.opts.keepalive,
	}
	c.cli = http.Client{
		Transport: tr,
		Timeout:   c.opts.timeout,
	}
	return nil
}

func loadBalancer(method string, max int) func() int {
	var m sync.Mutex
	i := 0
	switch method {
	default:
		fallthrough
	case "none":
		return func() int {
			return 0
		}
	case "rr": //round robin
		return func() int {
			m.Lock()
			v := i
			i++
			m.Unlock()
			return v % max
		}
	case "rand":
		return func() int { // the global rand is thread safety
			return rand.Intn(max)
		}
	}
}

func (c httpClient) Request() error {
	url := c.opts.urls[c.lb()]
	req, err := http.NewRequest(c.opts.method, url, bytes.NewReader([]byte(c.opts.body)))
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", c.opts.userAgent)

	resp, err := c.cli.Do(req)
	if err != nil {
		return err
	}
	// Read to EOF, then the connection could be cached(keepalive) for next use
	// See details: https://serholiu.com/go-http-client-keepalive
	io.Copy(ioutil.Discard, resp.Body)
	return resp.Body.Close()
}
func init() {
	fperf.Register("http", newHTTPClient, "HTTP performance benchmark client")
}
