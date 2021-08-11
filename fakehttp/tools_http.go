package fakehttp

import (
	"bytes"
//	"encoding/binary"
	"net"
	"net/http"
	"net/http/httputil"
	"io"
	"sync"
//	"sync/atomic"
)

type HttpWritter struct {
	cl *Client
	dieLock       sync.Mutex
	die chan struct{}
	hasData chan []byte
}
func (c *HttpWritter) worker(token string) {
	var buf bytes.Reader
	var data []byte
	var ok bool
	for {
		select{
		case data, ok = <-c.hasData:
		}
		buf.Reset(data)
		c.doReq(token, &buf, !ok)
		if !ok {
			return
		}
	}
}
func (c *HttpWritter) doReq(token string, buf *bytes.Reader, cls bool) error {
	cl := c.cl
	req, err := http.NewRequest("POST", cl.getURL(), buf)
	if err != nil {
		Vlogln(2, "HttpWritter.Write() NewRequest err:", err)
		return err
	}

	req.Header.Set("User-Agent", cl.UserAgent)
	req.Header.Set("Cookie", cl.TokenCookieB + "=" + token + "; " + cl.TokenCookieC + "=" + cl.TxFlag)
	// req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("Connection", "keep-alive")
	if cls {
		req.Header.Set("X-EOF", token)
	}
	//req.Close = true

//	dump, err := httputil.DumpRequestOut(req, true)
//	Vlogln(2, "[dbg]HttpWritter:", string(dump), err)

	res, err := cl.Do(req)
	if err != nil {
		Vlogln(2, "HttpWritter.Write() send Request err:", err)
		return err
	}
	defer res.Body.Close()
	_, err = io.ReadAll(res.Body)
	if err != nil {
		Vlogln(2, "HttpWritter.Write() ReadAll err:", err)
	}
//	Vlogln(2, "HttpWritter.Write() http version:", res.Proto, buf.Size(), buf.Len())
	return err
}

func (c *HttpWritter) Write(data []byte) (n int, err error) {
	Vlogln(5, "HttpWritter.Write() pipe data:", len(data))
	sz := len(data)
	buf := make([]byte, sz, sz)
	copy(buf, data)
	select{
	case c.hasData <- buf:
	case <-c.die:
		return 0, io.EOF
	}
	return len(data), nil
}

func (c *HttpWritter) Close() error {
	c.dieLock.Lock()

	select {
	case <-c.die:
		c.dieLock.Unlock()
		return io.EOF
	default:
		close(c.die)
		close(c.hasData)
		c.dieLock.Unlock()
		return nil
	}
}

func NewHttpWritter(cl *Client, token string) (io.WriteCloser) {
	conn := &HttpWritter{
		cl: cl,
		die: make(chan struct{}),
		hasData: make(chan []byte, 1),
	}
	go conn.worker(token)
	return conn
}

func mkconn2(p1 net.Conn, rxChunked bool, p2 io.WriteCloser, txChunked bool, rbuf []byte) (net.Conn){
	rem := bytes.NewReader(rbuf)
	r := io.MultiReader(rem, p1)
	if rxChunked {
		r = httputil.NewChunkedReader(r)
		paddingReader := &PaddingReader{
			R: r,
			canRead: make(chan struct{}, 1),
		}
		go paddingReader.worker()
		r = paddingReader
	}
	rc := CloseableReader{ r, p1 }
	pipe := Conn {
		R: rc,
		W: p2,
		RefR: p1,
		RefW: p1,
	}
	if txChunked {
		pipe.W = httputil.NewChunkedWriter(p2)
	}
	return pipe
}

