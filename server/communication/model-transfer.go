package communication

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bqnic/diplomski-projekt/common"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	peer "github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
)

var modelProtocolID protocol.ID = "/fl/model/1.0.0"

func HandleModelProtocol(host host.Host, modelRoot string) {
	host.SetStreamHandler(modelProtocolID, func(stream network.Stream) {
		defer stream.Close()

		remote := stream.Conn().RemotePeer()
		log.Printf("[protocol] incoming stream from %s\n", remote)

		reader := bufio.NewReader(stream)

		reqLine, err := reader.ReadString('\n')
		if err != nil {
			log.Printf("failed reading request: %v\n", err)
			return
		}

		reqLine = strings.TrimSpace(reqLine)
		// request format: "GET <modelID>\n"
		parts := strings.SplitN(reqLine, " ", 2)
		if len(parts) != 2 || strings.ToUpper(parts[0]) != "GET" {
			io.WriteString(stream, "ERR invalid request\n")
			return
		}

		modelID := parts[1]
		path := filepath.Join(modelRoot, modelID)
		file, err := os.Open(path)
		if err != nil {
			io.WriteString(stream, fmt.Sprintf("ERR open: %v\n", err))
			log.Printf("could not open model %s: %v\n", path, err)
			return
		}
		defer file.Close()

		io.WriteString(stream, "OK\n") // handshake
		// stream file bytes
		n, err := io.Copy(stream, file)
		if err != nil {
			log.Printf("error sending model: %v\n", err)
			return
		}

		log.Printf("sent %d bytes of model %s to %s\n", n, modelID, remote)
	})
}

func GetRemoteModel(sub *pubsub.Subscription) {
	for {
			msg, err := sub.Next(common.Ctx)
			if err != nil {
				if common.Ctx.Err() != nil {
					return
				}
				log.Printf("error reading pubsub message: %v\n", err)
				continue
			}

			// ignore self published messages
			if msg.ReceivedFrom == common.Host.ID() {
				continue
			}

			var m common.ModelMeta
			if err := json.Unmarshal(msg.Data, &m); err != nil {
				log.Printf("invalid meta from %s: %v\n", msg.ReceivedFrom, err)
				continue
			}
			log.Printf("[pubsub] discovered model announcement: peer=%s model=%s size=%d\n", m.PeerID, m.ModelID, m.Size)

			// try to fetch it (simple: fetch immediately once)
			go func(meta common.ModelMeta) {
				peerID, err := peer.Decode(meta.PeerID)
				if err != nil {
					log.Printf("invalid peer id: %v\n", err)
					return
				}
				// ensure we have addresses for the peer. If not, try to dial (mDNS likely supplied addr)
				ctx2, cancel := context.WithTimeout(common.Ctx, 10*time.Second)
				defer cancel()

				if err := common.Host.Connect(ctx2, peer.AddrInfo{ID: peerID}); err != nil {
					log.Printf("connect failed to %s: %v\n", meta.PeerID, err)
				}

				// open stream
				stream, err := common.Host.NewStream(ctx2, peerID, modelProtocolID)
				if err != nil {
					log.Printf("open stream failed: %v\n", err)
					return
				}
				defer stream.Close()
				// request model
				_, err = stream.Write([]byte("GET " + meta.ModelID + "\n"))
				if err != nil {
					log.Printf("write request err: %v\n", err)
					return
				}

				br := bufio.NewReader(stream)
				line, err := br.ReadString('\n')
				if err != nil {
					log.Printf("failed read handshake: %v\n", err)
					return
				}

				if strings.HasPrefix(line, "OK") {
					outpath := filepath.Join("shared/", "remote_"+meta.ModelID)
					out, err := os.Create(outpath)
					if err != nil {
						log.Printf("create file err: %v\n", err)
						return
					}
					defer out.Close()
					n, err := io.Copy(out, br)
					if err != nil {
						log.Printf("copy err: %v\n", err)
						return
					}
					log.Printf("saved %d bytes to %s\n", n, outpath)
				} else {
					log.Printf("peer responded: %s", strings.TrimSpace(line))
				}
			}(m)
		}
}