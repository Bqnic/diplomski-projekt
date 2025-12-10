package pubsub

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
)

func NewAnnouncer() *Announcer {
	return &Announcer{
		announced: make(map[string]struct{}),
	}
}

func (a *Announcer) has(hash string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	_, ok := a.announced[hash]
	return ok
}

func (a *Announcer) add(hash string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.announced[hash] = struct{}{}
}

func hashFile(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()

	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}