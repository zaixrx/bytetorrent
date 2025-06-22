# ByteTorrent

> ⚠️ **Not production-ready.** This is a showcase with educational value, not a secure, fully-featured file sharing client.

![Preview](showcase.gif)

**ByteTorrent** is a minimal BitTorrent-style peer-to-peer file transfer prototype built in Go using the [`gop2p`](https://github.com/zaixrx/gop2p) library. It was implemented in a really short period (~1 day so do expect a lot of bugs!) as a demonstration of how `gop2p` can be used to coordinate communication between peers in a decentralized manner.

This project showcases:
- Chunked file distribution from a "host" to multiple peers
- Cooperative chunk fetching (each peer helps others)
- Event-driven peer communication using `gop2p`'s messaging system
- Peer discovery and broadcast via a central broadcaster

## Features

- File splitting into fixed-size chunks (1024 bytes)
- On-demand chunk segment exchange (`gimme`, `hereyago`)
- Peer coordination using a simple handshake protocol
- Hosting and joining of P2P pools


## Getting Started

### Prerequisites

- Go 1.20+
- Git

### Installation

```bash
git clone https://github.com/zaixrx/bytetorrent
cd ./bytetorrent
go build -o bytetorrent
```

#### Running the broadcaster
```bash
go run ./server
```

#### Running the host
```bash
./bytetorrent -path ./path/to/file_to_share
```

### Running other peers
```bash
./bytetorrent
```
