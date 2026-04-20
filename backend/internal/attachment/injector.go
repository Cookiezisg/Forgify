package attachment

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/schema"
)

// InjectIntoMessages prepends extracted file content and/or multimodal image parts
// into the last user message in the history.
func InjectIntoMessages(messages []*schema.Message, files []*FileInfo) ([]*schema.Message, error) {
	if len(files) == 0 {
		return messages, nil
	}

	var textParts []string
	var imageParts []schema.MessageInputPart

	for _, f := range files {
		if f.Kind == "image" {
			b64 := base64.StdEncoding.EncodeToString(f.Content)
			imageParts = append(imageParts, schema.MessageInputPart{
				Type: schema.ChatMessagePartTypeImageURL,
				Image: &schema.MessageInputImage{
					MessagePartCommon: schema.MessagePartCommon{
						Base64Data: &b64,
						MIMEType:   MIMEType(f.Name),
					},
				},
			})
			continue
		}

		content, err := Extract(f)
		if err != nil {
			return nil, fmt.Errorf("读取文件 %s 失败: %w", f.Name, err)
		}
		textParts = append(textParts,
			fmt.Sprintf("[附件: %s]\n%s\n[附件结束]", f.Name, content))
	}

	// Find the last user message and inject
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != schema.User {
			continue
		}

		prefix := strings.Join(textParts, "\n\n")
		originalContent := messages[i].Content

		if len(imageParts) > 0 {
			// Build multimodal message with text + images
			var parts []schema.MessageInputPart

			// Add images first
			parts = append(parts, imageParts...)

			// Add text content (file extractions + original message)
			var fullText string
			if prefix != "" {
				fullText = prefix + "\n\n" + originalContent
			} else {
				fullText = originalContent
			}
			parts = append(parts, schema.MessageInputPart{
				Type: schema.ChatMessagePartTypeText,
				Text: fullText,
			})

			messages[i].UserInputMultiContent = parts
			messages[i].Content = "" // cleared when using multimodal
		} else if prefix != "" {
			messages[i].Content = prefix + "\n\n" + originalContent
		}
		break
	}

	return messages, nil
}
