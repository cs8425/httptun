package main

import (
	"bufio"
	"math/rand"
	"net"
	"net/http"
	"sync"
	"time"
	"io"

	"log"
	"flag"
	"os"
	"runtime"
)

var verbosity int = 2

var port = flag.String("p", ":4040", "http bind port")
var target = flag.String("t", "127.0.0.1:5002", "real server")
var dir = flag.String("d", "./www", "web/file server root dir")

var tokenCookieA = flag.String("ca", "cna", "token cookie name A")
var tokenCookieB = flag.String("cb", "_tb_token_", "token cookie name B")
var headerServer = flag.String("hdsrv", "nginx", "http header: Server")

type server struct {
	mx       sync.Mutex
	die      chan struct{}
	states   map[string]*state
}

type state struct {
	IP       string
	mx       sync.Mutex
	connR    net.Conn
	bufR     *bufio.ReadWriter
	connW    net.Conn
	ttl      time.Time
}

var copyBuf sync.Pool

var srv server

var handlerFile http.Handler

// set cookie: tokenCookieA = XXXX
// try get cookie: tokenCookieB = XXXX
func handler(w http.ResponseWriter, r *http.Request) {
	var cc *state
	var ok bool

	c, err := r.Cookie(*tokenCookieB)
	if err != nil {
		Vlogln(3, "cookie err:", c, err)
		goto FILE
	}

	cc, ok = checkToken(c.Value)
	if ok {
		if r.Method == "GET" {
			Vlogln(2, "get check:", c.Value)
		} else if r.Method == "POST" {
			Vlogln(2, "post check:", c.Value)
		} else {
			goto FILE
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			goto FILE
		}
		header := w.Header()
		header.Set("Cache-Control", "private, no-store, no-cache, max-age=0")
		header.Set("Content-Encoding", "gzip")
		flusher.Flush()
		Vlogln(3, "Flush")

		hj, ok := w.(http.Hijacker)
		if !ok {
			Vlogln(2, "hijacking err1:", ok)
			return
		}
		Vlogln(3, "hijacking ok1")

		conn, bufrw, err := hj.Hijack()
		if err != nil {
			Vlogln(2, "hijacking err:", err)
			return
		}
		Vlogln(3, "hijacking ok2")

		cc.mx.Lock()
		defer cc.mx.Unlock()
		if r.Method == "GET" {
			Vlogln(2, c.Value, " -> client")
			cc.connW = conn
		}
		if r.Method == "POST" {
			Vlogln(2, c.Value, " <- client")
			cc.connR = conn
			cc.bufR = bufrw
		}
		if cc.connR != nil && cc.connW != nil {
			rmToken(c.Value)
			p0, err := net.DialTimeout("tcp", *target, 5*time.Second)
			if err != nil {
				Vlogln(2, "connect to:", *target, err)
				return
			}
			Vlogln(2, "connect ok:", *target)
			n := cc.bufR.Reader.Buffered()
			buf := make([]byte, n)
			cc.bufR.Reader.Read(buf[:n])
			p0.Write(buf[:n])
			Vlogln(4, "post flushed...", n, buf[:n])
			go 	cp(cc.connR, p0, cc.connW)
		}
		return
	}

FILE:
	header := w.Header()
	header.Set("Server", *headerServer)
	token := RandStringBytes(16)
	expiration := time.Now().AddDate(0, 0, 3)
	cookie := http.Cookie{Name: *tokenCookieA, Value: token, Expires: expiration}
	http.SetCookie(w, &cookie)
	regToken(token)

	Vlogln(2, "web:", r.URL.Path, token, c)

	handlerFile.ServeHTTP(w, r)
}


func main() {
	runtime.GOMAXPROCS(runtime.NumCPU() + 2)
	rand.Seed(int64(time.Now().Nanosecond()))
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

	handlerFile = http.FileServer(http.Dir(*dir))

	copyBuf.New = func() interface{} {
		return make([]byte, 4096)
	}

	srv.states = make(map[string]*state)
	go tokenCleaner(srv)

	http.HandleFunc("/", handler)
	http.Serve(lis, nil)

}

func regToken(token string) {
	srv.mx.Lock()
	defer srv.mx.Unlock()

	_, ok := srv.states[token]
	if ok {
		Vlogln(2, "dobule token err:", token)
	}
	srv.states[token] = &state {
		ttl: time.Now().Add(30 * time.Second),
	}
}
func checkToken(token string) (*state, bool) {
	srv.mx.Lock()
	defer srv.mx.Unlock()

	c, ok := srv.states[token]
	if !ok {
		return nil, false
	}
	if time.Now().After(c.ttl) {
		delete(srv.states, token)
		return nil, false
	}
	return c, true
}
func rmToken(token string) {
	srv.mx.Lock()
	defer srv.mx.Unlock()

	_, ok := srv.states[token]
	if !ok {
		return
	}

	delete(srv.states, token)

	return
}

func tokenCleaner(srv server) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-srv.die:
			return
		case <-ticker.C:
		}

		srv.mx.Lock()
		for idx, c := range srv.states {
			if time.Now().After(c.ttl) {
				delete(srv.states, idx)
			}
		}
		srv.mx.Unlock()
	}
}

func cp(p1 io.ReadCloser, p0 io.ReadWriteCloser, p2 io.WriteCloser) {
	defer p1.Close()
	defer p2.Close()
	defer p0.Close()

	// start tunnel
	p1die := make(chan struct{})
	go func() {
		buf := copyBuf.Get().([]byte)
		io.CopyBuffer(p0, p1, buf)
		close(p1die)
		copyBuf.Put(buf)
	}()

	p2die := make(chan struct{})
	go func() {
		buf := copyBuf.Get().([]byte)
		io.CopyBuffer(p2, p0, buf)
		close(p2die)
		copyBuf.Put(buf)
	}()

	// wait for tunnel termination
	select {
	case <-p1die:
	case <-p2die:
	}
}

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789/-_"
func RandStringBytes(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return string(b)
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

