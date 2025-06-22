package main

import (
	"os"
	"fmt"
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
	DataChunkSize uint32 = 1024
)

func main() {
	var (
		err error
		chunks []Chunk
	)

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
		log.Println("closing because of SIGINT signal")
	}()
	defer func () {	
		cancel()
		br.Close()
	}()

	pool := get_pool_from_br(br)	
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

	app_state := new_app_state(handle, peers, pool, chunks)
	app_state.handle_pool_p2p()
}

func get_pool_from_br(br *broadcast.Handle) *broadcast.PublicPool {
	pools, err := br.GetPoolIDs()
	if err != nil  {
		log.Fatal(err)
	}

	var pool *broadcast.PublicPool
	log.Printf("available pools: %v\n", pools)

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

type t_chunk_bounds struct {
	paddr string // address coressponding to the peer expecting to send this data
	chunk_index, start, size uint32 // 0-based
}

type t_client struct {
	peer *p2p.Peer
	read_bytes uint32
}

func new_client(peer *p2p.Peer) *t_client {
	return &t_client{
		peer: peer,
		read_bytes: 0,
	}
}

func new_clients(peers map[string]*p2p.Peer) map[string]*t_client {
	clients := make(map[string]*t_client)
	for k, p := range peers {
		clients[k] = new_client(p)
	}
	return clients 
}

type t_app_state struct {
	handle *p2p.Handle
	pool *p2p.PublicPool
	
	clients map[string]*t_client

	chunks []Chunk
	max_bytes, read_bytes uint32

	chunk_segs_to_recieve map[string]*t_chunk_bounds
}

func new_app_state(handle *p2p.Handle, peers map[string]*p2p.Peer, pool *p2p.PublicPool, chunks []Chunk) *t_app_state {
	return &t_app_state{
		pool: pool,
		handle: handle,

		clients: new_clients(peers),

		chunks: chunks,
		read_bytes: 0,
		max_bytes: 0,

		chunk_segs_to_recieve: make(map[string]*t_chunk_bounds),
	}
}

func (s *t_app_state) handle_pool_p2p() error {
	// peers that are already there in the network before
	peers, err := s.handle.ConnectToPool(s.pool)
	if err != nil {
		log.Println(err)
	}
	s.clients = new_clients(peers)
	for k := range s.clients {
		s.initialize_client(k)
	}

	// peers that are new to the network 
	port, err := extract_port(s.pool.YourIP)
	if err != nil {
		return err
	}
	if err := s.handle.Listen(port); err != nil {
		return err
	}
	for {
		p, err := s.handle.Accept()
		if err != nil {
			log.Printf("PEER_ACCEPT_FAILED: %s\n", err)
			break
		}
		s.clients[p.Addr] = new_client(p)
		s.initialize_client(p.Addr)
		
		greeting_packet := p2p.NewPacket()
		greeting_packet.WriteByte(bool_to_cbyte(s.pool.YourIP == s.pool.HostIP))
		
		var (
			ln uint32 = uint32(len(s.chunks))
			nbytes uint32 = uint32(len(s.chunks[ln - 1]))
		)
		if ln > 1 {
			nbytes += (ln - 1) * DataChunkSize
		}

		greeting_packet.WriteUint32(nbytes)

		p.Send("greeting", greeting_packet)
	}

	return err
}

func (s *t_app_state) initialize_client(caddr string) error {
	c, exists := s.clients[caddr]
	if !exists {
		return fmt.Errorf("client doesn't exist")
	}

	c.peer.On(p2p.PeerDisconnected, func(p *p2p.Packet) {
		log.Printf("peer %s disconnected", caddr)
		if caddr == s.pool.HostIP {
			log.Printf("Ajri ajri l host rah 5rj!")
			// TODO: terminate p2p handle
		}
		delete(s.clients, caddr)
	})

	c.peer.On("greeting", func(p *p2p.Packet) {
		is_host, err := p.ReadByte()
		if err != nil {
			return
		}
		read_bytes, err := p.ReadUInt32()
		if err != nil {
			return
		}
		c.read_bytes = read_bytes 
		if is_host == 1 {
			s.chunks = make([]Chunk, 0)
			s.max_bytes = read_bytes

			err := s.request_next_chunk_segs()
			if err != nil {
				log.Println("ERROR_OUGA_BOUGA:", err)
			}
		}
		log.Printf("peer %s has %d chunks! host? %d\n", caddr, c.read_bytes / DataChunkSize, is_host)
	})

	c.peer.On("gimme", func(packet *p2p.Packet) {
		chunk_index, err := packet.ReadUInt32()
		if err != nil {
			return
		}
		start, err := packet.ReadUInt32()
		if err != nil {
			return
		}
		size, err := packet.ReadUInt32()
		if err != nil {
			return
		}
		chunk_seg, err := s.has_chunk_segment(chunk_index, start, size);
		if err != nil {
			log.Printf("DONT_HAVE_SEG %v\n", err)
			return // maybe even send back an error to the peer that demanded the segment 
		}

		s_packet := p2p.NewPacket()
		s_packet.WriteUint32(chunk_index)
		s_packet.WriteUint32(start)
		s_packet.WriteUint32(size)
		s_packet.WriteBytes(chunk_seg)

		c.peer.Send("hereyago", s_packet)
	})

	c.peer.On("hereyago", func(packet *p2p.Packet) {
		chunk_index, err := packet.ReadUInt32()
		if err != nil {
			return
		}
		start, err := packet.ReadUInt32()
		if err != nil {
			return
		}
		size, err := packet.ReadUInt32()
		if err != nil {
			return
		}
		if err := s.expecting_chunk_seg(caddr, chunk_index, start, size); err != nil {
			log.Printf("UNVALID_HEREYAGO_RESPONSE: %s\n", err)
			return
		}
		dat, err := packet.Get(size)
		if err != nil {
			log.Println(err)
			return
		}

		s.add_chunk_segment(chunk_index, start, dat)
		s.read_bytes += uint32(len(dat))

		// recieved all segments for the current chunk
		if len(s.chunk_segs_to_recieve) == 0 {
			if s.read_bytes >= s.max_bytes {
				s.read_bytes = s.max_bytes
				log.Println("recieved all chunks!")
				return
			}

			packet := p2p.NewPacket()
			packet.WriteUint32(s.read_bytes)

			s.broadcast("newchunk", packet) 
			s.request_next_chunk_segs()
		}
	})

	c.peer.On("newchunk", func (p *p2p.Packet) {
		// if you want to hack this, you could start here ;)
		// unless I implement e2e encryption(which won't happen)
		nbr, err := p.ReadUInt32()
		if err != nil {
			return
		}
		s.clients[c.peer.Addr].read_bytes = nbr
		log.Printf("peer %s now has %d read_bytes!\n", c.peer.Addr, s.clients[c.peer.Addr].read_bytes)
	})

	s.handle.HandlePeerIO(c.peer)
	
	return nil
}

func (s *t_app_state) has_chunk_segment(chunk_index, start, size uint32) ([]byte, error) {
	if chunk_index >= uint32(len(s.chunks))  {
		return nil, fmt.Errorf("chunk_index(%d) went out of bound(%d)", chunk_index, len(s.chunks))
	}
	if start + size > uint32(len(s.chunks[chunk_index])) {
		return nil, fmt.Errorf("start(%d) + size(%d) went out of bound(%d)", start, size, len(s.chunks[chunk_index]))
	}
	return ([]byte)(s.chunks[chunk_index][start:start+size]), nil
}

// notice: this method removes the chunk segments bounds(csb) after operating on it regardless of it's validity
func (s *t_app_state) expecting_chunk_seg(caddr string, chunk_index, start, size uint32) error {
	csb, exists := s.chunk_segs_to_recieve[caddr]
	// exists is false only if a dumb peer randomly decided to send data
	// that it wasn't asked to send, or I fucked up(it is likely to be second case)
	if !exists {
		return fmt.Errorf("didn't expect %s to send chunk seg", caddr)
	}
	delete(s.chunk_segs_to_recieve, caddr)
	if csb.start != start || csb.size != size || csb.chunk_index != chunk_index {
		csbb := t_chunk_bounds{
			paddr: caddr,
			start: start,
			size: size,
			chunk_index: chunk_index,
		}
		return fmt.Errorf("first: %v, second %v", *csb, csbb)
	}
	return nil 
}

func (s *t_app_state) add_chunk_segment(chunk_index, start uint32, dat []byte) {
	copy(s.chunks[chunk_index][start:], dat)
}

func (s *t_app_state) broadcast(msg string, packet *p2p.Packet) {
	for _, c := range s.clients {
		c.peer.Send(msg, packet)
	}
}

// the only constraint is that you must request one seg from one donar peer
func (s *t_app_state) request_next_chunk_segs() error {
	if s.read_bytes >= s.max_bytes {
		return fmt.Errorf("recieved all %d chunks", s.max_bytes % DataChunkSize)
	}

	if len(s.chunk_segs_to_recieve) > 0 {
		return fmt.Errorf("must recieve every expected chunk segment from actively connected peers to advance")
		// TODO: disconnected peers and not active peers must be ignored
		// here is another vulnerability (; (unless I imeplement timeouts for enqueued requests)
	}

	s.chunks = append(s.chunks, make(Chunk, DataChunkSize))

	// TODO: targets need to be pre-computed
	targets := make([]string, 0)
	for k, c := range s.clients {
		if s.read_bytes >= c.read_bytes { continue }
		targets = append(targets, k)
	}

	ulen := uint32(len(targets))
	chunk_size := min(DataChunkSize, s.max_bytes - s.read_bytes) 
	seg_size := chunk_size / ulen

	log.Printf("targets to read %d bytes from: %v", seg_size, targets)

	for _i, taddr := range targets {
		i := uint32(_i)

		csb := &t_chunk_bounds{
			paddr: taddr,
			chunk_index: s.read_bytes / DataChunkSize,
			start: seg_size * i,
			size: seg_size,
		}

		if ulen == i + 1 && chunk_size % ulen != 0 {
			csb.size += chunk_size - seg_size * ulen
		}

		s.chunk_segs_to_recieve[csb.paddr] = csb
		log.Printf("CSB: %v %d", csb, s.read_bytes)

		packet := p2p.NewPacket()
		packet.WriteUint32(csb.chunk_index)
		packet.WriteUint32(csb.start)
		packet.WriteUint32(csb.size)

		s.clients[csb.paddr].peer.Send("gimme", packet)
	}

	log.Printf("chunk segs to recieve %v\n", s.chunk_segs_to_recieve)
	
	return nil
}
 
// Utils
func extract_port(addr string) (uint16, error) {
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return 0, err
	}
	port, err := strconv.Atoi(portStr)
	return uint16(port), err
}
func bool_to_cbyte(val bool) byte {
	if val { return 1 }
	return 0
}
