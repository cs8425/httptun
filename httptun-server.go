package main

import (
	"net"
	"net/http"
	"sync"
	"time"
	"io"

	"log"
	"flag"
	"os"
	"runtime"

	"./fakehttp"
)

var verbosity int = 2

var copyBuf sync.Pool

var port = flag.String("p", ":4040", "http bind port")
var target = flag.String("t", "127.0.0.1:5002", "real server")
var dir = flag.String("d", "./www", "web/file server root dir")

var tokenCookieA = flag.String("ca", "cna", "token cookie name A")
var tokenCookieB = flag.String("cb", "_tb_token_", "token cookie name B")
var tokenCookieC = flag.String("cc", "_cna", "token cookie name C")
var headerServer = flag.String("hdsrv", "nginx", "http header: Server")
var wsObf = flag.Bool("usews", false, "fake as websocket")

func handleClient(p1 net.Conn) {
//	Vlogln(3, "stream opened")
//	defer Vlogln(3, "stream closed")
	defer p1.Close()

	p2, err := net.DialTimeout("tcp", *target, 5*time.Second)
	if err != nil {
		Vlogln(2, "connect to:", *target, err)
		return
	}
	defer p2.Close()
	cp(p1, p2)
	Vlogln(2, "close", p1.RemoteAddr())
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU() + 2)
	flag.Parse()

	lis, err := net.Listen("tcp", *port)
	if err != nil {
		Vlogln(2, "Error listening:", err.Error())
		os.Exit(1)
	}
	defer lis.Close()

	Vlogln(2, "listening on:", lis.Addr())
	Vlogln(2, "target:", *target)
	Vlogln(2, "dir:", *dir)
	Vlogln(2, "token cookie A:", *tokenCookieA)
	Vlogln(2, "token cookie B:", *tokenCookieB)
	Vlogln(2, "token cookie C:", *tokenCookieC)
	Vlogln(2, "use ws:", *wsObf)

	copyBuf.New = func() interface{} {
		return make([]byte, 4096)
	}

	websrv := fakehttp.NewServer(lis)
	websrv.UseWs = *wsObf
	websrv.HttpHandler = http.FileServer(http.Dir(*dir))
	websrv.StartServer()

	for {
		if conn, err := websrv.Accept(); err == nil {
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

