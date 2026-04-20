package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"encoding/base64"
)

// FlightFields is one arrival or departure leg as stored in tourists.flight_data.
type FlightFields struct {
	FlightNumber string `json:"flight_number"`
	Date         string `json:"date"`    // DD.MM.YYYY
	Time         string `json:"time"`    // HH:MM
	Airport      string `json:"airport"` // e.g. "TOKYO NARITA"
}

// TicketFlights is the parser output — both legs in one struct.
type TicketFlights struct {
	Arrival   FlightFields `json:"arrival"`
	Departure FlightFields `json:"departure"`
}

const ticketSystemPrompt = `You are a flight-ticket parser for a Japanese visa agency. Given one or more ticket scans, extract the arrival leg INTO Japan and the return leg FROM Japan. Return ONLY valid JSON matching the schema below — no prose, no markdown.

OUTPUT SCHEMA:
{
  "arrival":   { "flight_number": "...", "date": "DD.MM.YYYY", "time": "HH:MM", "airport": "CITY AIRPORT" },
  "departure": { "flight_number": "...", "date": "DD.MM.YYYY", "time": "HH:MM", "airport": "CITY AIRPORT" }
}

RULES:
- arrival: the LAST leg that lands in Japan (for multi-leg itineraries, the final leg; date is Japan local).
- departure: the FIRST leg leaving Japan (takeoff from Japan; date is Japan local).
- If the ticket is strictly ONE-WAY (no return), leave all departure.* fields "".
- Airport format: "CITY AIRPORTNAME" in CAPS (e.g. "TOKYO NARITA", "OSAKA KANSAI").
- Flight number: include space, e.g. "SU 262", "CZ 8101".
- All dates DD.MM.YYYY.
- Never invent data. Missing → "".`

// ParseTicket sends the given ticket files (PDF/JPG/PNG) to Claude and
// returns the extracted flight data.
func ParseTicket(ctx context.Context, apiKey string, files []FileInput) (TicketFlights, error) {
	contents, err := buildFileContents(files)
	if err != nil {
		return TicketFlights{}, err
	}
	contents = append(contents, anthropicContent{
		Type: "text",
		Text: "Extract the flight data from the scan(s) above per the schema.",
	})

	req := anthropicRequest{
		Model:       ModelOpusParser,
		MaxTokens:   1024,
		Temperature: 0,
		System:      ticketSystemPrompt,
		Messages:    []anthropicMessage{{Role: "user", Content: contents}},
	}
	raw, err := callClaude(ctx, apiKey, req)
	if err != nil {
		return TicketFlights{}, fmt.Errorf("ticket parse call: %w", err)
	}
	var out TicketFlights
	if err := json.Unmarshal([]byte(extractJSON(raw)), &out); err != nil {
		return TicketFlights{}, fmt.Errorf("ticket parse decode: %w — raw: %s", err, raw)
	}
	return out, nil
}

// buildFileContents converts FileInput slices into Anthropic content blocks.
// Images → "image", PDFs/others → "document".
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
