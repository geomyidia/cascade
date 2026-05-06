package golist

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

// parseStream reads `go list -json` output (a sequence of whitespace-
// separated JSON objects, NOT a JSON array) and returns the decoded
// packages in stream order. Returns nil, nil on empty input.
//
// On decode failure, returns nil and a *ParseError carrying the failure
// offset, the buffered + remaining payload at that point (capped at
// ParseErrorMaxPayload), and the underlying json error as Cause.
//
// io.EOF is the normal terminator, not an error. io.ErrUnexpectedEOF
// (mid-record cutoff) is reported as a *ParseError with that wrapped
// in Cause.
func parseStream(r io.Reader) ([]Package, error) {
	dec := json.NewDecoder(r)
	var pkgs []Package
	for {
		var p Package
		if err := dec.Decode(&p); err != nil {
			if errors.Is(err, io.EOF) {
				return pkgs, nil
			}
			return nil, &ParseError{
				Offset:  dec.InputOffset(),
				Payload: capturePayload(dec.Buffered(), r),
				Cause:   err,
			}
		}
		pkgs = append(pkgs, p)
	}
}

// capturePayload pulls up to ParseErrorMaxPayload bytes from the
// decoder's buffered tail plus any remaining input, for inclusion in
// ParseError.Payload. Best-effort: any read errors during capture are
// swallowed — the capture is diagnostic, not load-bearing.
func capturePayload(buffered io.Reader, remaining io.Reader) string {
	var buf bytes.Buffer
	_, _ = io.CopyN(&buf, io.MultiReader(buffered, remaining), int64(ParseErrorMaxPayload))
	return buf.String()
}
