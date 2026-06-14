![](https://raw.githubusercontent.com/shiyanhui/dht/master/doc/screen-shot.png)

在这个视频上你可以看到爬取效果[Youtube](https://www.youtube.com/watch?v=AIpeQtw22kc).

## Introduction

DHT实现了BitTorrent DHT协议，主要包括：

- [BEP-3 (部分)](http://www.bittorrent.org/beps/bep_0003.html)
- [BEP-5](http://www.bittorrent.org/beps/bep_0005.html)
- [BEP-9](http://www.bittorrent.org/beps/bep_0009.html)
- [BEP-10](http://www.bittorrent.org/beps/bep_0010.html)

它包含两种模式，标准模式和爬虫模式。标准模式遵循DHT协议，你可以把它当做一个标准
的DHT组件。爬虫模式是为了嗅探到更多torrent文件信息，它在某些方面不遵循DHT协议。
基于爬虫模式，你可以打造你自己的[BTDigg](http://btdigg.org/)。

[bthub.io](http://bthub.io)是一个基于这个爬虫而建的BT搜索引擎，你可以把他当做
BTDigg的替代品。

## Installation

    go get github.com/maxs98/dht

## Example

下面是一个简单的爬虫例子，你可以到[这里](https://github.com/maxs98/dht/blob/master/sample)看完整的Demo。

```go
import (
    "fmt"
    "github.com/maxs98/dht"
)

func main() {
    downloader := dht.NewWire(65536)
    go func() {
        // once we got the request result
        for resp := range downloader.Response() {
            fmt.Println(resp.InfoHash, resp.MetadataInfo)
        }
    }()
    go downloader.Run()

    config := dht.NewCrawlConfig()
    config.OnAnnouncePeer = func(infoHash, ip string, port int) {
        // request to download the metadata info
        downloader.Request([]byte(infoHash), ip, port)
    }
    d := dht.New(config)

    d.Run()
}
```

### NAT 穿透 (新增)

爬虫现在支持在 NAT 后面运行（例如局域网或家庭路由器后面），通过 STUN 协议发现公网 IP 和端口，
并定期发送 keepalive 保持 UDP 端口映射存活。

**用法：**

```go
// 自动 STUN 发现
config := dht.NewNATCrawlConfig()

// 手动指定公网 IP（如果你知道的话）
config := dht.NewNATCrawlConfigWithIP("1.2.3.4", 6881)

// 或手动配置
config := dht.NewCrawlConfig()
config.NATConfig = &dht.NATConfig{
    Enabled:     true,
    STUNServers: []string{"stun.l.google.com:19302"},
}

d := dht.New(config)

// 打印发现的公网地址
log.Printf("Public address: %s", d.PublicAddr())
```

**原理：**
1. 查询公共 STUN 服务器（默认 Google 的）发现公网 IP:port
2. 将发现地址用于 DHT 通信
3. 每 25 秒发送 keepalive ping 保持 NAT 映射存活
4. STUN 发现失败时自动降级为纯出站模式——爬虫仍可正常运行

**CLI 参数（spider 示例）：**
```bash
go run sample/spider/spider.go --nat
go run sample/spider/spider.go --public-ip=1.2.3.4 --public-port=6881
```

## 注意

- 默认的爬虫配置需要300M左右内存，你可以根据你的服务器内存大小调整MaxNodes和
  BlackListMaxSize
- ✅ 现已支持 NAT 穿透（基于 STUN RFC 5389），使用 `NewNATCrawlConfig()` 即可

## TODO

- [x] NAT穿透，在局域网内也能够运行
- [ ] 完整地实现BEP-3，这样不但能够下载种子，也能够下载资源
- [ ] 优化

## Blog

你可以在[这里](https://github.com/shiyanhui/dht/wiki)看到DHT Spider教程。

## License

[MIT](https://github.com/maxs98/dht/blob/master/LICENSE)
