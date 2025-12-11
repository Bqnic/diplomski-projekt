package common

// ModelMeta is published to the pubsub topic so peers can learn who has what
type ModelMeta struct {
	PeerID   string `json:"peer_id"`
	ModelID  string `json:"model_id"`
	Size     int64  `json:"size"`	
	Time     int64  `json:"time_unix"`
	Hash     string `json:"hash"`
}