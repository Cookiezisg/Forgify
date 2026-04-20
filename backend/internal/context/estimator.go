package context

import "github.com/cloudwego/eino/schema"

// EstimateTokens approximates the token count for a message list.
// Uses character-count / 3.5 heuristic (within ~10% for mixed CJK/Latin).
func EstimateTokens(messages []*schema.Message) int {
	total := 0
	for _, m := range messages {
		total += len([]rune(m.Content))
		// Multimodal content: estimate each image at ~1000 tokens
		total += len(m.UserInputMultiContent) * 1000
	}
	return total * 10 / 35 // ≈ /3.5
}
