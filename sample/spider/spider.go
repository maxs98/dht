package main

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/maxs98/dht"
	"log"
	"net/http"
	_ "net/http/pprof"
)

type file struct {
	Path   []interface{} `json:"path"`
	Length int           `json:"length"`
}

type bitTorrent struct {
	InfoHash string `json:"infohash"`
	Name     string `json:"name"`
	Files    []file `json:"files,omitempty"`
	Length   int    `json:"length,omitempty"`
}

func main() {
	natFlag := flag.Bool("nat", false, "Enable NAT traversal via STUN")
	publicIP := flag.String("public-ip", "", "Manual public IP (overrides STUN)")
	publicPort := flag.Int("public-port", 0, "Manual public port")
	flag.Parse()

	go func() {
		http.ListenAndServe(":6060", nil)
	}()

	w := dht.NewWire(65536, 1024, 256)
	go func() {
		for resp := range w.Response() {
			metadata, err := dht.Decode(resp.MetadataInfo)
			if err != nil {
				continue
			}
			info := metadata.(map[string]interface{})

			if _, ok := info["name"]; !ok {
				continue
			}

			bt := bitTorrent{
				InfoHash: hex.EncodeToString(resp.InfoHash),
				Name:     info["name"].(string),
			}

			if v, ok := info["files"]; ok {
				files := v.([]interface{})
				bt.Files = make([]file, len(files))

				for i, item := range files {
					f := item.(map[string]interface{})
					bt.Files[i] = file{
						Path:   f["path"].([]interface{}),
						Length: f["length"].(int),
					}
				}
			} else if _, ok := info["length"]; ok {
				bt.Length = info["length"].(int)
			}

			data, err := json.Marshal(bt)
			if err == nil {
				fmt.Printf("%s\n\n", data)
			}
		}
	}()
	go w.Run()

	var config *dht.Config
	if *natFlag {
		config = dht.NewNATCrawlConfig()
		log.Println("NAT traversal enabled (STUN)")
	} else if *publicIP != "" {
		port := *publicPort
		if port == 0 {
			port = 6881
		}
		config = dht.NewNATCrawlConfigWithIP(*publicIP, port)
		log.Printf("NAT traversal enabled (manual: %s:%d)", *publicIP, port)
	} else {
		config = dht.NewCrawlConfig()
	}

	config.OnAnnouncePeer = func(infoHash, ip string, port int) {
		w.Request([]byte(infoHash), ip, port)
	}
	d := dht.New(config)

	// Print public address if NAT is enabled
	if addr := d.PublicAddr(); addr != nil {
		log.Printf("Public address: %s", addr.String())
	}

	d.Run()
}
