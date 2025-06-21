package main

import (
	"os"
	"flag"
	"log"
	"net"
	"os/signal"
	"context"
	"strconv"

	p2p "github.com/zaixrx/gop2p/core"
	"github.com/zaixrx/gop2p/core/broadcast"
)

const (
	BRPort = 6969
	BRPingTicks = 20
	BRHostname = "127.0.0.1"
	DataChunkSize = 1024
)

type Client struct {
	peer *p2p.Peer

	chunks []Chunk
	obtainedChunks uint32 
}

func NewClient(p *p2p.Peer) *Client {
	return &Client{
		peer: p,
		chunks: make([]Chunk, 0),
		obtainedChunks: 0,
	}
}

func main() {
	blobPath := flag.String("path", "", "path for the blob to be sent over the p2p network")
	flag.Parse()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	ctx, cancel := context.WithCancel(context.Background())

	br := broadcast.CreateHandle(ctx)
	if err := br.Connect(BRHostname, BRPort); err != nil {
		log.Panic("Could not connect to br:", err)
	}

	go func() {
		<-c
		cancel()
	}()
	defer func () {	
		br.Close()
		cancel()
	}()

	pool := get_pool_from_br(br)

	var (
		chunks []Chunk
		err error
	)
	if pool.HostIP == pool.YourIP {
		chunks, err = get_file_chunks(*blobPath)
		if err != nil {
			log.Fatal("couldn't get file chunks", err)
		}

		go br.Ping(ctx, BRPingTicks)
	}
	
	peers := make(map[string]*p2p.Peer)
	handle := p2p.CreateHandle(ctx, func() {
		packet := p2p.NewPacket()

		for _, p := range peers {
			p.Send(p2p.PeerDisconnected, packet)
		}
	})

	handle_pool_p2p(handle, peers, pool, chunks)
}

func get_pool_from_br(br *broadcast.Handle) *broadcast.PublicPool {
	pools, err := br.GetPoolIDs()
	if err != nil  {
		log.Fatal(err)
	}

	var pool *broadcast.PublicPool
	log.Println(pools)

	if len(pools) > 0 {
		pool, err = br.JoinPool(pools[0])
		if err != nil {
			log.Fatal("Join pool", err)
		}
	} else {
		pool, err = br.CreatePool()
		if err != nil {
			log.Fatal("Create Pool", err)
		}
	}

	return pool
}

func handle_pool_p2p(handle *p2p.Handle, peers map[string]*p2p.Peer, pool *broadcast.PublicPool, chunks []Chunk) {
	var (
		err error
		clients = make(map[string]*Client)
		obtainedChunks uint32 = uint32(len(chunks))
	)

	peers, err = handle.ConnectToPool(pool)
	if err != nil {
		log.Println(err)
	}

	var handlePeer = func(paddr string) {
		p, exists := peers[paddr]
		if !exists {
			return
		}

		client := NewClient(p) 

		p.On(p2p.PeerDisconnected, func(p *p2p.Packet) {
			log.Printf("peer %s disconnected", paddr)
			if paddr == pool.HostIP {
				log.Printf("Ajri ajri l host rah 5rj!")
			}
			delete(peers, paddr)
		})

		p.On("greeting", func(p *p2p.Packet) {
			obtainedChunks, err := p.ReadUInt32()
			if err != nil {
				return
			}
			client.obtainedChunks = obtainedChunks
			log.Printf("peer %s has %d chunks!", paddr, obtainedChunks)
		})	

		clients[paddr] = client
		handle.HandlePeerIO(p)
	}

	// Peers that are already there
	for _, p := range peers {
		handlePeer(p.Addr)
	}

	port, err := extract_port(pool.YourIP)
	if err != nil {
		log.Fatal(err)
	}

	if handle.Listen(port) != nil {
		log.Fatal(err)
	}

	for {
		// New peers
		p, err := handle.Accept()
		if err != nil {
			log.Println("Accept Failed", err)
			break
		}
		peers[p.Addr] = p
		handlePeer(p.Addr)

		packet := p2p.NewPacket()
		packet.WriteUint32(obtainedChunks)

		p.Send("greeting", packet)
	}
}
 
func extract_port(addr string) (uint16, error) {
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return 0, err
	}
	port, err := strconv.Atoi(portStr)
	return uint16(port), err
}
