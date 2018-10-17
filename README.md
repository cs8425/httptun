# httptun

A Simple Tunnel let TCP connection looks like HTTP request.
Server side also with a simple web/file server.
For the normal browser, the server side is just a website.
Suggest using long connection(Multiplexing/VPN) with this tool.


#### ~~Warning: No encrypted connections between server and client !!!~~
#### ~~You should use TLS or something similar first, and then go through this tool.~~
#### now support HTTPS/TLS server!

### overhead
  * 3 http requset for initial
    * ws mode: 2 http requset
  * 2 tcp connection for data transmission
    * ws mode: 1 tcp connection

### Usage

```
Client: ./httptun-client -t "HTTPTUN_SERVER_IP:4040" -p ":5005"
Server: ./httptun-server -t "TARGET_IP:5005" -p ":4040"
```
The above commands will establish port forwarding for 8388/tcp as:

> Application -> **httptun Client(5005/tcp) -> httptun Server(4040/http)** -> Target Server(5005/tcp)

Tunnels the original connection:

> Application -> Target Server(5005/tcp)


### Code Usage

server side:

```golang
	// simple http Handler setup
	fileHandler := http.FileServer(http.Dir(*dir))
	//http.Handle("/", fileHandler) // do not add to http.DefaultServeMux now
	http.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) { // other Handler
		io.WriteString(w, "Hello, world!\n")
	})

	websrv := fakehttp.NewHandle(fileHandler) // bind handler
	websrv.UseWs = true
	websrv.OnlyWs = false
	http.Handle("/", websrv) // now add to http.DefaultServeMux

	// start http server
	http.ListenAndServe(":8080", nil)
```

