package main

import (
	"net"
	"flag"
	"io"
	"sync"
	"os"
	"runtime"
	"log"

	"./fakehttp"
)

var verbosity int = 2

var copyBuf sync.Pool

var port = flag.String("p", "127.0.0.1:5005", "bind port")
var target = flag.String("t", "127.0.0.1:4040", "http server address & port")
var targetUrl = flag.String("url", "/", "http url to send")

var tokenCookieA = flag.String("ca", "cna", "token cookie name A")
var tokenCookieB = flag.String("cb", "_tb_token_", "token cookie name B")
var tokenCookieC = flag.String("cb", "_cna", "token cookie name C")

var userAgent = flag.String("ua", "Mozilla/5.0 (Windows NT 10.0; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/47.0.2526.80 Safari/537.36 QQBrowser/9.3.6874.400", "User-Agent (default: QQ)")

var wsObf = flag.Bool("usews", false, "fake as websocket")

func handleClient(p1 net.Conn) {
	defer p1.Close()

	cl := fakehttp.NewClient(*target)
	cl.TokenCookieA = *tokenCookieA
	cl.TokenCookieB = *tokenCookieB
	cl.TokenCookieC = *tokenCookieC
	cl.UseWs = *wsObf
	cl.UserAgent = *userAgent
	cl.Url = *targetUrl
	p2, err := cl.Dial()
	if err != nil {
		Vlogln(2, "Dial err:", err)
		return
	}
	defer p2.Close()
	cp(p1, p2)
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU() + 2)
	flag.Parse()

	if *tokenCookieA == *tokenCookieB {
		Vlogln(2, "Error: token cookie cannot bee same!")
		os.Exit(1)
	}

	lis, err := net.Listen("tcp", *port)
	if err != nil {
		Vlogln(2, "Error listening:", err.Error())
		os.Exit(1)
	}
	defer lis.Close()

	Vlogln(2, "listening on:", lis.Addr())
	Vlogln(2, "target:", *target)
	Vlogln(2, "token cookie A:", *tokenCookieA)
	Vlogln(2, "token cookie B:", *tokenCookieB)
	Vlogln(2, "token cookie C:", *tokenCookieC)
	Vlogln(2, "use ws:", *wsObf)

	copyBuf.New = func() interface{} {
		return make([]byte, 4096)
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

