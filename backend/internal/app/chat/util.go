package chat

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/cloudwego/eino/schema"

	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
)

func newMsgID() string        { return "msg_" + randHex(8) }
func newAttachmentID() string { return "att_" + randHex(8) }

func randHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("chat: crypto/rand failed: %v", err))
	}
	return hex.EncodeToString(b)
}

func imageToInputPart(att *chatdomain.Attachment, provider string) (schema.MessageInputPart, error) {
	if !supportsVision(provider) {
		return schema.MessageInputPart{}, chatdomain.ErrVisionNotSupported
	}
	data, err := os.ReadFile(att.StoragePath)
	if err != nil {
		return schema.MessageInputPart{}, fmt.Errorf("%w: %v", chatdomain.ErrAttachmentParseFailed, err)
	}
	encoded := base64.StdEncoding.EncodeToString(data)
	return schema.MessageInputPart{
		Type: schema.ChatMessagePartTypeImageURL,
		Image: &schema.MessageInputImage{
			MessagePartCommon: schema.MessagePartCommon{
				Base64Data: &encoded,
				MIMEType:   att.MimeType,
			},
		},
	}, nil
}

func supportsVision(provider string) bool {
	switch provider {
	case "openai", "anthropic", "google":
		return true
	default:
		return false
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
