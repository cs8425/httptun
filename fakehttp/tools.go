package fakehttp

import (
	"bytes"
	"encoding/binary"
	"math/rand"
	"net"
	"net/http/httputil"
	"io"
	"log"
	"time"
	"sync"
)

const verbosity int = 2

type Conn struct {
	R     io.ReadCloser
	W     io.WriteCloser
	RefR  net.Conn
	RefW  net.Conn
}
func (c Conn) Read(data []byte) (n int, err error)  { return c.R.Read(data) }
func (c Conn) Write(data []byte) (n int, err error) { return c.W.Write(data) }

func (c Conn) Close() error {
	if err := c.W.Close(); err != nil {
		return err
	}
	if err := c.R.Close(); err != nil {
		return err
	}
	return nil
}

func (c Conn) LocalAddr() net.Addr {
	if ts, ok := c.RefW.(interface {
		LocalAddr() net.Addr
	}); ok {
		return ts.LocalAddr()
	}
	return nil
}

func (c Conn) RemoteAddr() net.Addr {
	if ts, ok := c.RefW.(interface {
		RemoteAddr() net.Addr
	}); ok {
		return ts.RemoteAddr()
	}
	return nil
}

func (c Conn) SetReadDeadline(t time.Time) error {
	return c.RefR.SetWriteDeadline(t)
}

func (c Conn) SetWriteDeadline(t time.Time) error {
	return c.RefW.SetWriteDeadline(t)
}

func (c Conn) SetDeadline(t time.Time) error {
	if err := c.SetReadDeadline(t); err != nil {
		return err
	}
	if err := c.SetWriteDeadline(t); err != nil {
		return err
	}
	return nil
}

type CloseableReader struct {
	io.Reader
	r0     io.ReadCloser
}
func (c CloseableReader) Close() error {
	return c.r0.Close()
}

func mkconn(p1 net.Conn, rxChunked bool, p2 net.Conn, txChunked bool, rbuf []byte) (net.Conn){
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
		RefW: p2,
	}
	if txChunked {
		pipe.W = httputil.NewChunkedWriter(p2)
	}
	return pipe
}

type PaddingReader struct {
	mx sync.Mutex
	R io.Reader
	buffer bytes.Buffer
	cls bool
	canRead chan struct{}
}
func (c *PaddingReader) worker() {
	const HdrSz = 4
	buf := make([]byte, 65535)
	cls := false
	for {
		_, err := io.ReadFull(c.R, buf[0:HdrSz])
		if err != nil { // TODO
			cls = true
			c.mx.Lock()
			c.cls = true
			c.mx.Unlock()

			select{
			case c.canRead <- struct{}{}:
			default:
			}
			return
		}
		paddingSz := binary.LittleEndian.Uint16(buf[0:])
		if paddingSz <= HdrSz {
			continue
		}

		dataSz := binary.LittleEndian.Uint16(buf[2:])
		_, err = io.ReadFull(c.R, buf[:paddingSz-HdrSz])
		if err != nil { // TODO
			Vlogln(5, "[dbg]PaddingReader:", err)

			cls = true
			c.mx.Lock()
			c.cls = true
			c.mx.Unlock()
		}
		if dataSz > 0 || cls {
			Vlogln(5, "PaddingReader.worker() data:", dataSz)
			c.mx.Lock()
			c.buffer.Write(buf[:dataSz])
			c.mx.Unlock()

			select{
			case c.canRead <- struct{}{}:
			default:
			}
		}
	}
}
func (c *PaddingReader) Read(data []byte) (n int, err error)  {
	select {
	case <-c.canRead:
		c.mx.Lock()
		n, err = c.buffer.Read(data)
		cls := c.cls
		c.mx.Unlock()
		if cls {
			return n, err
		} else {
			return n, nil
		}
	}
}
func (c *PaddingReader) Close() error {
	Vlogln(5, "[dbg]PaddingReader close")
	c.mx.Lock()
	c.cls = true
	c.mx.Unlock()
	return nil
}


type ConnAddr struct {
	net.Conn //io.WriteCloser
	Addr string
}
func (c *ConnAddr) RemoteAddr() net.Addr {
	return (*StrAddr)(c)
}

type StrAddr ConnAddr
func (c *StrAddr) Network() string {
	return c.Conn.RemoteAddr().Network()
}
func (c *StrAddr) String() string {
	if c == nil {
		return "<nil>"
	}
	if c.Addr == "" {
		return c.Conn.RemoteAddr().String()
	}
	return c.Addr
}

func mkConnAddr(p1 net.Conn, address string) (net.Conn) {
	if address != "" {
		conn := &ConnAddr{
			Conn: p1,
			Addr: address,
		}
		return conn
	}
	return p1
}

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789/-_"
func randStringBytes(n int) string {
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


