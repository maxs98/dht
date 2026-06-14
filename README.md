![](https://raw.githubusercontent.com/shiyanhui/dht/master/doc/screen-shot.png)

See the video on the [Youtube](https://www.youtube.com/watch?v=AIpeQtw22kc).

[中文版README](https://github.com/maxs98/dht/blob/master/README_CN.md)

## Introduction

DHT implements the bittorrent DHT protocol in Go. Now it includes:

- [BEP-3 (part)](http://www.bittorrent.org/beps/bep_0003.html)
- [BEP-5](http://www.bittorrent.org/beps/bep_0005.html)
- [BEP-9](http://www.bittorrent.org/beps/bep_0009.html)
- [BEP-10](http://www.bittorrent.org/beps/bep_0010.html)

It contains two modes, the standard mode and the crawling mode. The standard
mode follows the BEPs, and you can use it as a standard dht server. The crawling
mode aims to crawl as more metadata info as possiple. It doesn't follow the
standard BEPs protocol. With the crawling mode, you can build another [BTDigg](http://btdigg.org/).

[bthub.io](http://bthub.io) is a BT search engine based on the crawling mode.

## Installation

    go get github.com/maxs98/dht

## Example

Below is a simple spider. You can move [here](https://github.com/maxs98/dht/blob/master/sample)
to see more samples.

```go
import (
    "fmt"
    "github.com/maxs98/dht"
)

func main() {
    downloader := dht.NewWire(65535)
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

### NAT Traversal (NEW)

The crawler can now run behind NAT (e.g. in a LAN or behind a home router) using STUN-based
NAT traversal. This discovers your public IP and port mapping, and keeps the UDP port
mapping alive with periodic keepalive pings.

**Usage:**

```go
// Automatic STUN discovery
config := dht.NewNATCrawlConfig()

// Manual public IP (if you know it)
config := dht.NewNATCrawlConfigWithIP("1.2.3.4", 6881)

// Or configure manually
config := dht.NewCrawlConfig()
config.NATConfig = &dht.NATConfig{
    Enabled:     true,
    STUNServers: []string{"stun.l.google.com:19302"},
}

d := dht.New(config)

// Print the discovered public address
log.Printf("Public address: %s", d.PublicAddr())
```

**How it works:**
1. Queries public STUN servers (Google's by default) to discover your public IP:port
2. Uses the discovered address for DHT communication (announce_peer, etc.)
3. Sends periodic keepalive pings (every 25s) to keep the NAT mapping alive
4. Falls back to outbound-only mode if STUN discovery fails — the crawler still works

**CLI flags (spider sample):**
```bash
go run sample/spider/spider.go --nat
go run sample/spider/spider.go --public-ip=1.2.3.4 --public-port=6881
```

## Note

- The default crawl mode configure costs about 300M RAM. Set **MaxNodes**
  and **BlackListMaxSize** to fit yourself.
- ✅ NAT traversal is now supported via STUN (RFC 5389). Use `NewNATCrawlConfig()`.
- DHT routing table entries on remote nodes use the public IP discovered via STUN.

## TODO

- [x] NAT Traversal.
- [ ] Implements the full BEP-3.
- [ ] Optimization.

## FAQ

#### Why it is slow compared to other spiders ?

Well, maybe there are several reasons.

- DHT aims to implements the standard BitTorrent DHT protocol, not born for crawling the DHT network.
- It will block ip which looks like bad and a good ip may be mis-judged.

## License

MIT, read more [here](https://github.com/maxs98/dht/blob/master/LICENSE)
