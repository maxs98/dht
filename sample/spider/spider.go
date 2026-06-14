package main

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/maxs98/dht"
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
	logFile := flag.String("log", "", "Write output to file (JSONL)")
	statsInterval := flag.Int("stats", 30, "Print stats every N seconds (0=off)")
	flag.Parse()

	// Write JSONL to file if requested
	var fileWriter *os.File
	if *logFile != "" {
		var err error
		fileWriter, err = os.OpenFile(*logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			log.Fatalf("Cannot open log file: %v", err)
		}
		defer fileWriter.Close()
		log.Printf("Writing results to %s", *logFile)
	}

	// pprof on :6060
	go func() {
		log.Println("pprof: http://localhost:6060/debug/pprof/")
		http.ListenAndServe(":6060", nil)
	}()

	// Counters
	var (
		totalAnnounced atomic.Int64
		totalRequested atomic.Int64
		totalDownloaded atomic.Int64
	)

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

			totalDownloaded.Add(1)
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
			} else if l, ok := info["length"]; ok {
				bt.Length = l.(int)
			}

			data, err := json.Marshal(bt)
			if err != nil {
				continue
			}

			line := string(data)
			fmt.Println(line)

			if fileWriter != nil {
				fileWriter.WriteString(line + "\n")
			}
		}
	}()
	go w.Run()

	var config *dht.Config
	if *natFlag {
		config = dht.NewNATCrawlConfig()
		log.Println("✅ NAT traversal enabled (STUN)")
	} else if *publicIP != "" {
		port := *publicPort
		if port == 0 {
			port = 6881
		}
		config = dht.NewNATCrawlConfigWithIP(*publicIP, port)
		log.Printf("✅ NAT traversal enabled (manual: %s:%d)", *publicIP, port)
	} else {
		config = dht.NewCrawlConfig()
	}

	config.OnGetPeers = func(infoHash, ip string, port int) {
		totalAnnounced.Add(1)
	}

	config.OnAnnouncePeer = func(infoHash, ip string, port int) {
		totalAnnounced.Add(1)
		totalRequested.Add(1)
		w.Request([]byte(infoHash), ip, port)
	}

	d := dht.New(config)

	// Print public address
	if addr := d.PublicAddr(); addr != nil {
		log.Printf("📡 Public address: %s", addr.String())
	}

	// Stats goroutine
	if *statsInterval > 0 {
		go func() {
			ticker := time.NewTicker(time.Duration(*statsInterval) * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				log.Printf("📊 announced=%d | requested=%d | downloaded=%d",
					totalAnnounced.Load(),
					totalRequested.Load(),
					totalDownloaded.Load(),
				)
			}
		}()
	}

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Printf("\n📊 Final: announced=%d requested=%d downloaded=%d",
			totalAnnounced.Load(), totalRequested.Load(), totalDownloaded.Load())
		os.Exit(0)
	}()

	log.Println("🚀 DHT crawler started — waiting for first metadata download (may take 5-15 min)...")
	d.Run()
}
