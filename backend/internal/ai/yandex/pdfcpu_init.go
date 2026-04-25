// Package init for the pdfcpu library — disables its silent write of
// configuration files to the user's home directory. We use pdfcpu in
// memory only (Trim with bytes.Reader / bytes.Buffer); without this
// call pdfcpu would write ~/Library/Application Support/pdfcpu/...
// (config.yml, EU certs, fonts) on first OCR invocation, which:
//   - breaks read-only container filesystems
//   - pollutes CI sandboxes
//   - has a documented data race on first concurrent call
package yandex

import "github.com/pdfcpu/pdfcpu/pkg/api"

func init() {
	api.DisableConfigDir()
}
