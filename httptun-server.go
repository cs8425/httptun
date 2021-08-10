package main

import (
	"crypto/tls"
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
var wsObf = flag.Bool("usews", true, "fake as websocket")
var onlyWs = flag.Bool("onlyws", false, "only accept websocket")

var copyBuffSz = flag.Int("buffer", 16*1024, "copy buffer size (bytes)")

var crtFile    = flag.String("crt", "", "PEM encoded certificate file")
var keyFile    = flag.String("key", "", "PEM encoded private key file")

func handleClient(p1 net.Conn) {
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

	Vlogln(2, "listening on:", *port)
	Vlogln(2, "target:", *target)
	Vlogln(2, "dir:", *dir)
	Vlogln(2, "token cookie A:", *tokenCookieA)
	Vlogln(2, "token cookie B:", *tokenCookieB)
	Vlogln(2, "token cookie C:", *tokenCookieC)
	Vlogln(2, "use ws:", *wsObf)
	Vlogln(2, "only ws:", *onlyWs)

	copyBuf.New = func() interface{} {
		return make([]byte, *copyBuffSz)
	}


	// simple http Handler setup
	fileHandler := http.FileServer(http.Dir(*dir))
	//http.Handle("/", fileHandler) // do not add to http.DefaultServeMux now
	http.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) { // other Handler
		io.WriteString(w, "Hello, world!\n")
	})

	websrv := fakehttp.NewHandle(fileHandler) // bind handler
	websrv.UseWs = *wsObf
	websrv.OnlyWs = *onlyWs
	http.Handle("/", websrv) // now add to http.DefaultServeMux

	// start http server
	srv := &http.Server{Addr: *port, Handler: nil}
	go startServer(srv)

	// accept real connections
	for {
		if conn, err := websrv.Accept(); err == nil {
			Vlogln(2, "remote address:", conn.RemoteAddr())

			go handleClient(conn)
		} else {
			Vlogf(2, "%+v", err)
		}
	}
}

func byListener() {

	// start Listener
	lis, err := net.Listen("tcp", *port)
	if err != nil {
		Vlogln(2, "Error listening:", err.Error())
		os.Exit(1)
	}
	defer lis.Close()

	// setup fakehttp
	websrv := fakehttp.NewServer(lis)
	websrv.UseWs = *wsObf
	websrv.HttpHandler = http.FileServer(http.Dir(*dir))
	websrv.StartServer()

	// accept real connections
	for {
		if conn, err := websrv.Accept(); err == nil {
			Vlogln(2, "remote address:", conn.RemoteAddr())

			go handleClient(conn)
		} else {
			Vlogf(2, "%+v", err)
		}
	}
}

func startServer(srv *http.Server) {
	var err error

	// check tls
	if *crtFile != "" && *keyFile != "" {
		cfg := &tls.Config{
			MinVersion:               tls.VersionTLS12,
			CurvePreferences:         []tls.CurveID{tls.CurveP521, tls.CurveP384, tls.CurveP256},
			PreferServerCipherSuites: true,
			CipherSuites: []uint16{

				tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
				tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,

				tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,

				tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256, // http/2 must
				tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256, // http/2 must

				tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256,
				tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256,

				tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
				tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,

				tls.TLS_RSA_WITH_AES_256_GCM_SHA384, // weak
				tls.TLS_RSA_WITH_AES_256_CBC_SHA, // waek
			},
		}
		srv.TLSConfig = cfg
		//srv.TLSNextProto: make(map[string]func(*http.Server, *tls.Conn, http.Handler), 0) // disable http/2

		log.Printf("HTTPS server Listen on: %v", *port)
		err = srv.ListenAndServeTLS(*crtFile, *keyFile)
	} else {
		log.Printf("HTTP server Listen on: %v", *port)
		err = srv.ListenAndServe()
	}

	if err != http.ErrServerClosed {
		log.Printf("ListenAndServe error: %v", err)
		os.Exit(1)
	}
}

func cp(p1, p2 io.ReadWriteCloser) {
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

