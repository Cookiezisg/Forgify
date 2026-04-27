package chat

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
)

func newMsgID() string        { return "msg_" + randHex(8) }
func newBlockID() string      { return "blk_" + randHex(8) }
func newAttachmentID() string { return "att_" + randHex(8) }

func randHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("chat: crypto/rand failed: %v", err))
	}
	return hex.EncodeToString(b)
}

func encodeBase64(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// readAndEncode reads a file from disk and returns its base64-encoded content.
//
// readAndEncode 从磁盘读取文件并返回其 base64 编码内容。
func readAndEncode(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("readAndEncode: %w", err)
	}
	return encodeBase64(data), nil
}
