package fakehttp

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"
)

var (
	errBrokenPipe      = errors.New("broken pipe")
	ErrServerClose     = errors.New("server close")
)

type Server struct {
	mx            sync.Mutex
	die           chan struct{}
	dieLock       sync.Mutex
	states        map[string]*state
	accepts       chan net.Conn
	lis           net.Listener

	TxMethod      string
	RxMethod      string
	TxFlag        string
	RxFlag        string
	TokenCookieA  string
	TokenCookieB  string
	TokenCookieC  string
	HeaderServer  string
	HttpHandler   http.Handler
	UseWs         bool
	TokenTTL      time.Duration
}

type state struct {
	IP       string
	mx       sync.Mutex
	connR    net.Conn
	bufR     *bufio.ReadWriter
	connW    net.Conn
	ttl      time.Time
}

func NewServer(lis net.Listener) (*Server) {
	srv := &Server{
		lis: lis,
		states: make(map[string]*state),
		accepts: make(chan net.Conn, 128),
		TxMethod:     txMethod,
		RxMethod:     rxMethod,
		TxFlag:       txFlag,
		RxFlag:       rxFlag,
		TokenCookieA: tokenCookieA,
		TokenCookieB: tokenCookieB,
		TokenCookieC: tokenCookieC,
		HeaderServer: headerServer,
		HttpHandler: http.FileServer(http.Dir("./www")),
		UseWs: false,
		TokenTTL: tokenTTL,
	}

	return srv
}

func NewHandle(hdlr http.Handler) (*Server) {
	srv := &Server{
		states: make(map[string]*state),
		accepts: make(chan net.Conn, 128),
		TxMethod:     txMethod,
		RxMethod:     rxMethod,
		TxFlag:       txFlag,
		RxFlag:       rxFlag,
		TokenCookieA: tokenCookieA,
		TokenCookieB: tokenCookieB,
		TokenCookieC: tokenCookieC,
		HeaderServer: headerServer,
		HttpHandler: hdlr,
		UseWs: false,
		TokenTTL: tokenTTL,
	}

	go srv.tokenCleaner()

	return srv
}

func (srv *Server) StartServer() () {
	if srv.lis == nil {
		return
	}

	go srv.tokenCleaner()

	http.HandleFunc("/", srv.ServeHTTP)
	go http.Serve(srv.lis, nil)
}

func (srv *Server) Accept() (net.Conn, error) {
	select {
	case <-srv.die:
		return nil, ErrServerClose
	case conn := <-srv.accepts:
		return conn, nil
	}
}

func (srv *Server) Addr() (net.Addr) {
	if srv.lis == nil {
		return nil
	}
	return srv.lis.Addr()
}

func (srv *Server) Close() (error) {
	srv.dieLock.Lock()

	select {
	case <-srv.die:
		srv.dieLock.Unlock()
		return ErrServerClose
	default:
		close(srv.die)
		srv.dieLock.Unlock()
		if srv.lis != nil {
			return srv.lis.Close()
		}
		return nil
	}
}

func (srv *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var cc *state
	var ok bool
	var err error
	var c, ct *http.Cookie

	c, err = r.Cookie(srv.TokenCookieB)
	if err != nil {
		Vlogln(3, "cookieB err:", c, err)
		goto FILE
	}

	ct, err = r.Cookie(srv.TokenCookieC)
	if err != nil {
		Vlogln(3, "cookieC err:", ct, err)
		goto FILE
	}
	Vlogln(3, "cookieC ok:", ct)

	cc, ok = srv.checkToken(c.Value)
	if ok {
		if r.Method == srv.RxMethod || r.Method == srv.TxMethod {
			Vlogln(2, "req check:", c.Value)
		} else {
			goto FILE
		}

		if !srv.UseWs {
			flusher, ok := w.(http.Flusher)
			if !ok {
				goto FILE
			}
			header := w.Header()
			header.Set("Cache-Control", "private, no-store, no-cache, max-age=0")
			header.Set("Content-Encoding", "gzip")
			flusher.Flush()
			Vlogln(3, "Flush")
		}

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
		bufrw.Flush()

		if srv.UseWs {
			conn.Write([]byte("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: " + c.Value + "\r\n\r\n"))
		}

		cc.mx.Lock()
		defer cc.mx.Unlock()
		if r.Method == srv.RxMethod && ct.Value == srv.RxFlag {
			Vlogln(2, c.Value, " -> client")
			cc.connW = conn
		}
		if r.Method == srv.TxMethod && ct.Value == srv.TxFlag  {
			Vlogln(2, c.Value, " <- client")
			cc.connR = conn
			cc.bufR = bufrw
		}
		if cc.connR != nil && cc.connW != nil {
			srv.rmToken(c.Value)

			n := cc.bufR.Reader.Buffered()
			buf := make([]byte, n)
			cc.bufR.Reader.Read(buf[:n])
			srv.accepts <- mkconn(cc.connR, cc.connW, buf[:n])
		}
		Vlogln(3, "init end")
		return
	}

FILE:
	header := w.Header()
	header.Set("Server", srv.HeaderServer)
	token := randStringBytes(16)
	expiration := time.Now().AddDate(0, 0, 3)
	cookie := http.Cookie{Name: srv.TokenCookieA, Value: token, Expires: expiration}
	http.SetCookie(w, &cookie)
	srv.regToken(token)

	Vlogln(2, "web:", r.URL.Path, token, c)

	srv.HttpHandler.ServeHTTP(w, r)
}

func (srv *Server) regToken(token string) {
	srv.mx.Lock()
	defer srv.mx.Unlock()

	_, ok := srv.states[token]
	if ok {
		Vlogln(2, "dobule token err:", token)
	}
	srv.states[token] = &state {
		ttl: time.Now().Add(srv.TokenTTL),
	}
}
func (srv *Server) checkToken(token string) (*state, bool) {
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
func (srv *Server) rmToken(token string) {
	srv.mx.Lock()
	defer srv.mx.Unlock()

	_, ok := srv.states[token]
	if !ok {
		return
	}

	delete(srv.states, token)

	return
}

func (srv *Server) tokenCleaner() {
	ticker := time.NewTicker(tokenClean)
	defer ticker.Stop()
	for {
		select {
		case <-srv.die:
			return
		case <-ticker.C:
		}

		list := make([]*state, 0)

		srv.mx.Lock()
		for idx, c := range srv.states {
			if time.Now().After(c.ttl) {
				delete(srv.states, idx)
				list = append(list, c)
				Vlogln(4, "[gc]", idx, c)
			}
		}
		srv.mx.Unlock()

		// check and close half open connection
		for _, cc := range list {
			cc.mx.Lock()
			if cc.connR == nil && cc.connW != nil {
				cc.connW.Close()
				cc.connW = nil
				Vlogln(4, "[gc]half open W", cc)
			}
			if cc.connR != nil && cc.connW == nil {
				cc.connR.Close()
				cc.connR = nil
				Vlogln(4, "[gc]half open R", cc)
			}
			cc.mx.Unlock()
		}
	}
}

