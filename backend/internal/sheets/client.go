package sheets

import (
	"context"
	"fmt"
	"strings"
	"time"

	"fujitravel-admin/backend/internal/matcher"

	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

// nameColumns lists candidate column names to search in, in priority order.
// The sheet uses the long form; the short form is a fallback.
var nameColumns = []string{
	"ФИО (латиницей, как в загранпаспорте)",
	"ФИО (латиницей)",
	"ФИО латиницей",
}

// Client wraps the Google Sheets API for reading tourist data.
type Client struct {
	svc     *sheets.Service
	sheetID string
}

// New creates a new Sheets client authenticated via a service account JSON file.
func New(ctx context.Context, credentialsPath, sheetID string) (*Client, error) {
	creds, err := google.FindDefaultCredentials(ctx,
		"https://www.googleapis.com/auth/spreadsheets.readonly",
	)
	if err != nil {
		// Fall back to explicit credentials file.
		_ = creds
	}

	svc, err := sheets.NewService(ctx,
		option.WithCredentialsFile(credentialsPath),
		option.WithScopes("https://www.googleapis.com/auth/spreadsheets.readonly"),
	)
	if err != nil {
		return nil, fmt.Errorf("create sheets service: %w", err)
	}
	return &Client{svc: svc, sheetID: sheetID}, nil
}

// isTransient returns true for network-level errors that deserve a retry
// (Google OAuth endpoint occasionally returns EOF / reset mid-handshake).
func isTransient(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "EOF") ||
		strings.Contains(s, "connection reset") ||
		strings.Contains(s, "i/o timeout") ||
		strings.Contains(s, "connection refused") ||
		strings.Contains(s, "TLS handshake")
}

// AllRows fetches all rows from the first sheet and returns them as a slice of
// maps keyed by the header row values. Retries transient network errors.
func (c *Client) AllRows(ctx context.Context) ([]map[string]string, error) {
	var resp *sheets.ValueRange
	var err error
	backoff := 300 * time.Millisecond
	for attempt := 0; attempt < 4; attempt++ {
		resp, err = c.svc.Spreadsheets.Values.Get(c.sheetID, "A1:ZZ").Context(ctx).Do()
		if err == nil {
			break
		}
		if !isTransient(err) {
			return nil, fmt.Errorf("fetch sheet values: %w", err)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
		}
		backoff *= 2
	}
	if err != nil {
		return nil, fmt.Errorf("fetch sheet values after retries: %w", err)
	}

	if len(resp.Values) < 2 {
		return nil, nil
	}

	// First row is the header.
	header := make([]string, len(resp.Values[0]))
	for i, v := range resp.Values[0] {
		header[i] = fmt.Sprintf("%v", v)
	}

	rows := make([]map[string]string, 0, len(resp.Values)-1)
	for _, raw := range resp.Values[1:] {
		row := make(map[string]string, len(header))
		for i, h := range header {
			if i < len(raw) {
				row[h] = fmt.Sprintf("%v", raw[i])
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// AllRowsReversed returns all rows latest-first (last sheet row comes first).
func (c *Client) AllRowsReversed(ctx context.Context) ([]map[string]string, error) {
	rows, err := c.AllRows(ctx)
	if err != nil {
		return nil, err
	}
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}
	return rows, nil
}

// SearchByName performs a fuzzy search against the name column and returns the top n matches.
func (c *Client) SearchByName(ctx context.Context, query string, n int) ([]matcher.Match, error) {
	rows, err := c.AllRows(ctx)
	if err != nil {
		return nil, err
	}

	// Find which column name is actually present in the sheet.
	col := nameColumns[0] // default
	if len(rows) > 0 {
		for _, candidate := range nameColumns {
			if _, ok := rows[0][candidate]; ok {
				col = candidate
				break
			}
		}
	}

	return matcher.TopN(query, col, rows, n), nil
}
