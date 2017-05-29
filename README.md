# httptun



### Usage

```
Client: ./httptun-client -t "HTTPTUN_SERVER_IP:4040" -p ":5005"
Server: ./httptun-server -t "TARGET_IP:5005" -p ":4040"
```
The above commands will establish port forwarding for 8388/tcp as:

> Application -> **httptun Client(5005/tcp) -> httptun Server(4040/http)** -> Target Server(5005/tcp)

Tunnels the original connection:

> Application -> Target Server(5005/tcp)



