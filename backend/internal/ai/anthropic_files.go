package ai

import (
	"encoding/base64"
	"fmt"
	"path/filepath"
	"strings"
)

// buildFileContents converts FileInput slices into Anthropic content blocks.
// Images → "image", PDFs/others → "document".
//
// Used by voucher_parser.go (still on Anthropic until Task 1.C2). The ticket
// parser no longer needs this — it goes through Yandex Vision OCR + GPT.
// Kept in its own file so the ticket-parser source can shrink to just the
// new two-step pipeline.
func buildFileContents(files []FileInput) ([]anthropicContent, error) {
	var contents []anthropicContent
	for _, inp := range files {
		if inp.AnthropicFileID != "" {
			ext := strings.ToLower(filepath.Ext(inp.Name))
			blockType := "document"
			if ext == ".jpg" || ext == ".jpeg" || ext == ".png" {
				blockType = "image"
			}
			contents = append(contents, anthropicContent{
				Type:   blockType,
				Source: &contentSource{Type: "file", FileID: inp.AnthropicFileID},
			})
			continue
		}
		ext := strings.ToLower(filepath.Ext(inp.Name))
		switch ext {
		case ".jpg", ".jpeg":
			contents = append(contents, anthropicContent{
				Type: "image",
				Source: &contentSource{
					Type: "base64", MediaType: "image/jpeg",
					Data: base64.StdEncoding.EncodeToString(inp.Data),
				},
			})
		case ".png":
			contents = append(contents, anthropicContent{
				Type: "image",
				Source: &contentSource{
					Type: "base64", MediaType: "image/png",
					Data: base64.StdEncoding.EncodeToString(inp.Data),
				},
			})
		case ".pdf":
			contents = append(contents, anthropicContent{
				Type: "document",
				Source: &contentSource{
					Type: "base64", MediaType: "application/pdf",
					Data: base64.StdEncoding.EncodeToString(inp.Data),
				},
			})
		default:
			contents = append(contents, anthropicContent{
				Type: "text",
				Text: fmt.Sprintf("File: %s\n\n%s", inp.Name, string(inp.Data)),
			})
		}
	}
	return contents, nil
}
