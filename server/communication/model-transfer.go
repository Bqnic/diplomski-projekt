package communication

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bqnic/diplomski-projekt/common"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/network"
	peer "github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
)

var modelProtocolID protocol.ID = "/fl/model/1.0.0"

func HandleModelProtocol() {
	common.Host.SetStreamHandler(modelProtocolID, func(stream network.Stream) {
		defer stream.Close()

		remote := stream.Conn().RemotePeer()
		common.Log.Infow("Incoming model request", "peer_short", common.ShortID(remote.String()))

		reader := bufio.NewReader(stream)

		reqLine, err := reader.ReadString('\n')
		if err != nil {
			common.Log.Errorw("Failed reading request from stream", "peer_short", common.ShortID(remote.String()), "err", err)
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
		path := filepath.Join(common.LocalModelDir, modelID)
		file, err := os.Open(path)
		if err != nil {
			io.WriteString(stream, "ERR open: "+err.Error()+"\n")
			common.Log.Errorw("Could not open model file", "path", path, "err", err)
			return
		}
		defer file.Close()

		io.WriteString(stream, "OK\n") // handshake
		// stream file bytes
		n, err := io.Copy(stream, file)
		if err != nil {
			common.Log.Errorw("Error sending model to peer", "peer_short", common.ShortID(remote.String()), "err", err, "model", modelID)
			return
		}

		common.Log.Infow("Sent model to peer", "bytes", n, "model", modelID, "peer_short", common.ShortID(remote.String()))
	})
}

func GetRemoteModel(sub *pubsub.Subscription) {
	for {
			msg, err := sub.Next(common.Ctx)
				if err != nil {
					if common.Ctx.Err() != nil {
						return
					}
					common.Log.Errorw("Error reading pubsub message", "err", err)
				continue
			}

			// ignore self published messages
			if msg.ReceivedFrom == common.Host.ID() {
				continue
			}

			var m common.ModelMeta
			if err := json.Unmarshal(msg.Data, &m); err != nil {
				common.Log.Warnw("Invalid model metadata in announcement", "from_short", common.ShortID(msg.ReceivedFrom.String()), "err", err)
				continue
			}
			common.Log.Infow("Discovered model announcement", "peer_short", common.ShortID(m.PeerID), "model", m.ModelID, "size", m.Size)

			// try to fetch it (simple: fetch immediately once)
			go func(meta common.ModelMeta) {
				peerID, err := peer.Decode(meta.PeerID)
				if err != nil {
					common.Log.Warnw("Invalid peer id in announcement", "peer", common.ShortID(meta.PeerID), "err", err)
					return
				}
				// ensure we have addresses for the peer. If not, try to dial (mDNS likely supplied addr)
				ctx2, cancel := context.WithTimeout(common.Ctx, 10*time.Second)
				defer cancel()

				if err := common.Host.Connect(ctx2, peer.AddrInfo{ID: peerID}); err != nil {
					common.Log.Warnw("Could not connect to announcing peer", "peer_short", common.ShortID(meta.PeerID), "err", err)
				}

				// open stream
				stream, err := common.Host.NewStream(ctx2, peerID, modelProtocolID)
				if err != nil {
					common.Log.Errorw("Failed to open stream to peer", "peer_short", common.ShortID(meta.PeerID), "err", err)
					return
				}
				defer stream.Close()
				// request model
				_, err = stream.Write([]byte("GET " + meta.ModelID + "\n"))
				if err != nil {
					common.Log.Errorw("Failed to write model request to stream", "peer_short", common.ShortID(meta.PeerID), "err", err)
					return
				}

				br := bufio.NewReader(stream)
				line, err := br.ReadString('\n')
				if err != nil {
					common.Log.Errorw("Failed reading handshake from peer", "peer_short", common.ShortID(meta.PeerID), "err", err)
					return
				}

				if strings.HasPrefix(line, "OK") {
					outpath := filepath.Join(common.RemoteModelDir, meta.ModelID)
					out, err := os.Create(outpath)
					if err != nil {
						common.Log.Errorw("Failed creating local file for remote model", "path", outpath, "err", err)
						return
					}
					defer out.Close()
					n, err := io.Copy(out, br)
					if err != nil {
						common.Log.Errorw("Failed saving remote model to disk", "err", err, "path", outpath)
						return
					}
					common.Log.Infow("Saved remote model to disk", "bytes", n, "model", meta.ModelID)
				} else {
					common.Log.Infow("Peer responded with error", "resp", strings.TrimSpace(line), "peer_short", common.ShortID(meta.PeerID))
				}
			}(m)
		}
}