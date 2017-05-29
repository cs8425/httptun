package main

import (
	"bufio"
	"net"
    "net/http"
	"flag"
	"io"
	"io/ioutil"
	"sync"
	"time"

	"os"
	"runtime"
	"log"
)

var verbosity int = 2

var copyBuf sync.Pool

var port = flag.String("p", "127.0.0.1:5005", "bind port")
var target = flag.String("t", "127.0.0.1:4040", "http server address & port")
var targetUrl = flag.String("url", "/", "http url to send")

var tokenCookieA = flag.String("ca", "cna", "token cookie name A")
var tokenCookieB = flag.String("cb", "_tb_token_", "token cookie name B")

var userAgent = flag.String("ua", "Mozilla/5.0 (Windows NT 10.0; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/47.0.2526.80 Safari/537.36 QQBrowser/9.3.6874.400", "User-Agent (default: QQ)")

func getToken() (string) {
	client := &http.Client{
		Timeout: time.Second * 10,
	}

	req, err := http.NewRequest("GET", "http://" + *target, nil)
	if err != nil {
		Vlogln(2, "getToken() NewRequest err:", err)
		return ""
	}

	req.Header.Set("User-Agent", *userAgent)
	res, err := client.Do(req)
	if err != nil {
		Vlogln(2, "getToken() send Request err:", err)
		return ""
	}
	defer res.Body.Close()

	cookies := res.Cookies()

//	body, err := ioutil.ReadAll(res.Body)
	_, err = ioutil.ReadAll(res.Body)
	if err != nil {
		Vlogln(2, "getToken() ReadAll err:", err)
	}
//	Vlogln(2, "getToken()", cookies, string(body))
	Vlogln(3, "getToken()", cookies)

	for _, cookie := range cookies {
		Vlogln(3, "cookie:", cookie.Name, cookie.Value)
		if cookie.Name == *tokenCookieA {
			return cookie.Value
		}
	}

	return ""
}

func getTx(token string) (io.WriteCloser, []byte) {

	req, err := http.NewRequest("POST", "http://" + *target, nil)
	if err != nil {
		Vlogln(2, "getTx() NewRequest err:", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("User-Agent", *userAgent)
	req.Header.Set("Cookie", *tokenCookieB + "=" + token)


	tx, err := net.DialTimeout("tcp", *target, 5*time.Second)
	if err != nil {
		Vlogln(2, "Tx connect to:", *target, err)
		return nil, nil
	}
	Vlogln(3, "Tx connect ok:", *target)
	req.Write(tx)

	txbuf := bufio.NewReaderSize(tx, 1024)
	res, err := http.ReadResponse(txbuf, req)
	if err != nil {
		Vlogln(2, "Tx ReadResponse", err, res)
		return nil, nil
	}
//	Vlogln(2, "Tx Response", res, txbuf)

//	body := bufio.NewReader(res.Body)
//	Vlogln(2, "Tx Response", body.Buffered(), txbuf.Buffered())

	return tx, nil
}

func getRx(token string) (io.ReadCloser, []byte) {

	req, err := http.NewRequest("GET", "http://" + *target, nil)
	if err != nil {
		Vlogln(2, "getRx() NewRequest err:", err)
	}

	req.Header.Set("User-Agent", *userAgent)
	req.Header.Set("Cookie", *tokenCookieB + "=" + token)

	rx, err := net.DialTimeout("tcp", *target, 5*time.Second)
	if err != nil {
		Vlogln(2, "Rx connect to:", *target, err)
		return nil, nil
	}
	Vlogln(3, "Rx connect ok:", *target)
	req.Write(rx)

	rxbuf := bufio.NewReaderSize(rx, 1024)
	res, err := http.ReadResponse(rxbuf, req)
	if err != nil {
		Vlogln(2, "Rx ReadResponse", err, res)
		return nil, nil
	}
//	Vlogln(2, "Rx Response", res, rxbuf)

//	body := bufio.NewReader(res.Body)
//	Vlogln(2, "Rx Response", body.Buffered(), rxbuf.Buffered())

	n := rxbuf.Buffered()
	Vlogln(2, "Rx Response", n)
	if n > 0 {
		buf := make([]byte, n)
		rxbuf.Read(buf[:n])
		return rx, buf[:n]
	} else {
		return rx, nil
	}
}


func handleClient(p0 net.Conn) {
	defer p0.Close()

	token := getToken()
	Vlogln(2, "token:", token)

	if token == "" {
		return
	}

	tx, _ := getTx(token)
	if tx == nil {
		return
	}
	defer tx.Close()

	rx, rxbuf := getRx(token)
	if rx == nil {
		return
	}
	defer rx.Close()
	p0.Write(rxbuf)

	Vlogln(2, "tx:", tx)
	Vlogln(2, "rx:", rx)
	cp(rx, p0, tx)
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


func cp(p1 io.ReadCloser, p0 io.ReadWriteCloser, p2 io.WriteCloser) {
//	Vlogln(2, "stream opened")
//	defer Vlogln(2, "stream closed")
//	defer p1.Close()
//	defer p2.Close()
//	defer p0.Close()

	// start tunnel
	p1die := make(chan struct{})
	go func() {
		buf := copyBuf.Get().([]byte)
		io.CopyBuffer(p0, p1, buf)
/*
		for {
			n, err := p1.Read(buf)
			if err != nil {
				break
			}
			Vlogln(2, "p1 -> p0:", buf[:n])

			_, err = p0.Write(buf[:n])
			if err != nil {
				break
			}
		}
*/
		close(p1die)
		copyBuf.Put(buf)
	}()

	p2die := make(chan struct{})
	go func() {
		buf := copyBuf.Get().([]byte)
		io.CopyBuffer(p2, p0, buf)
/*
		for {
			n, err := p0.Read(buf)
			if err != nil {
				break
			}
			Vlogln(2, "p0 -> p2:", buf[:n])

			_, err = p2.Write(buf[:n])
			if err != nil {
				break
			}
		}
*/
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

