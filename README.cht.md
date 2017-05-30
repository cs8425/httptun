# httptun

一個簡單的小工具把普通TCP連線偽裝成HTTP請求
伺服器端包含一個簡單的web/file伺服器
對一般瀏覽器而言, 伺服器端只是個網站
建議使用長連線(Multiplexing/VPN之類)以減少overhead

#### 注意: 伺服器跟客戶端之間沒有任何加密
#### 請先用TLS或類似的東西加密完, 再通過這個工具


### overhead
  * 初始化: 3個HTTP請求
  * 資料傳輸: 2個tcp連線

### Usage

```
Client: ./httptun-client -t "HTTPTUN_SERVER_IP:4040" -p ":5005"
Server: ./httptun-server -t "TARGET_IP:5005" -p ":4040"
```
以上指令會對8388/tcp做轉發(port forwarding):

> Application -> **httptun Client(5005/tcp) -> httptun Server(4040/http)** -> Target Server(5005/tcp)

原始連線如下:

> Application -> Target Server(5005/tcp)




