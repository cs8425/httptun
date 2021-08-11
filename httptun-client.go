package main

import (
	"net"
	"net/url"
	"flag"
	"io"
	"io/ioutil"
	"sync"
	"os"
	"runtime"
	"log"

	"./fakehttp"
)

var (
	verbosity int = 2

	copyBuf sync.Pool
	cl *fakehttp.Client

	port = flag.String("p", "127.0.0.1:5005", "bind port")
	targetAddr = flag.String("t", "http://127.0.0.1:4040", "http server address & port")

	crtFile    = flag.String("crt", "", "PEM encoded certificate file")
	tlsVerify = flag.Bool("k", true, "InsecureSkipVerify")

	tokenCookieA = flag.String("ca", "cna", "token cookie name A")
	tokenCookieB = flag.String("cb", "_tb_token_", "token cookie name B")
	tokenCookieC = flag.String("cc", "_cna", "token cookie name C")

	userAgent = flag.String("ua", "Mozilla/5.0 (Windows NT 10.0; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/47.0.2526.80 Safari/537.36 QQBrowser/9.3.6874.400", "User-Agent (default: QQ)")

	copyBuffSz = flag.Int("buffer", 16*1024, "copy buffer size (bytes)")
)

func handleClient(p1 net.Conn) {
	defer p1.Close()

	p2, err := cl.Dial()
	if err != nil {
		Vlogln(2, "Dial err:", err)
		return
	}
	defer p2.Close()
	cp(p1, p2)
	Vlogln(2, "close", p1.RemoteAddr())
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU() + 2)
	flag.Parse()

	if *tokenCookieA == *tokenCookieB {
		Vlogln(2, "Error: token cookie cannot bee same!")
		os.Exit(1)
	}

	u, err := url.Parse(*targetAddr)
	if err != nil {
		Vlogln(2, "Error: parse target addr:", err)
		return
	}
	useWs := false
	useTLS := false
	targetUrl := u.Path
	target := u.Host
	switch u.Scheme {
	case "http":
		useWs = false
		useTLS = false
	case "https":
		useWs = false
		useTLS = true
	case "ws":
		useWs = true
		useTLS = false
	case "wss":
		useWs = true
		useTLS = true
	default:
		Vlogln(2, "Error: connection type not support", u.Scheme)
		return
	}

	lis, err := net.Listen("tcp", *port)
	if err != nil {
		Vlogln(2, "Error listening:", err.Error())
		os.Exit(1)
	}
	defer lis.Close()

	Vlogln(2, "listening on:", lis.Addr())
	Vlogln(2, "target:", target)
	Vlogln(2, "token cookie A:", *tokenCookieA)
	Vlogln(2, "token cookie B:", *tokenCookieB)
	Vlogln(2, "token cookie C:", *tokenCookieC)
	Vlogln(2, "use ws:", useWs)
	Vlogln(2, "use TLS:", useTLS)
	Vlogln(2, "use certificate:", *crtFile)

	if useTLS {
		if *crtFile != "" {
			caCert, err := ioutil.ReadFile(*crtFile)
			if err != nil {
				Vlogln(2, "Reading certificate error:", err)
				os.Exit(1)
			}
			cl = fakehttp.NewTLSClient(target, caCert, *tlsVerify)
		} else {
			cl = fakehttp.NewTLSClient(target, nil, *tlsVerify)
		}
	} else {
		cl = fakehttp.NewClient(target)
	}
	cl.TokenCookieA = *tokenCookieA
	cl.TokenCookieB = *tokenCookieB
	cl.TokenCookieC = *tokenCookieC
	cl.UseWs = useWs
	cl.UserAgent = *userAgent
	cl.Url = targetUrl

	copyBuf.New = func() interface{} {
		return make([]byte, *copyBuffSz)
	}

	for {
		if conn, err := lis.Accept(); err == nil {
			Vlogln(2, "remote address:", conn.RemoteAddr())

			go handleClient(conn)
		} else {
			Vlogf(2, "%+v", err)
		}
	}

}

func cp(p1, p2 io.ReadWriteCloser) {
//	Vlogln(2, "stream opened")
//	defer Vlogln(2, "stream closed")
//	defer p1.Close()
//	defer p2.Close()

	// start tunnel
	p1die := make(chan struct{})
	go func() {
		buf := copyBuf.Get().([]byte)
		io.CopyBuffer(p1, p2, buf)
		close(p1die)
		copyBuf.Put(buf)
	}()

	p2die := make(chan struct{})
	go func() {
		buf := copyBuf.Get().([]byte)
		io.CopyBuffer(p2, p1, buf)
		close(p2die)
		copyBuf.Put(buf)
	}()

	// wait for tunnel termination
	select {
	case <-p1die:
	case <-p2die:
	}
}
func Vlogf(level int, format string, v ...interface{}) {
	if level <= verbosity {
		log.Printf(format, v...)
	}
}
func Vlog(level int, v ...interface{}) {
	if level <= verbosity {
		log.Print(v...)
	}
}
func Vlogln(level int, v ...interface{}) {
	if level <= verbosity {
		log.Println(v...)
	}
}

