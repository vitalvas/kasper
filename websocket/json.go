package websocket

import (
	"encoding/json"
	"io"
)

// WriteJSON writes the JSON encoding of v as a message.
func (c *Conn) WriteJSON(v any) error {
	w, err := c.NextWriter(TextMessage)
	if err != nil {
		return err
	}
	err = json.NewEncoder(w).Encode(v)
	if closeErr := w.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	return err
}

// ReadJSON reads the next JSON-encoded message from the connection and
// stores it in the value pointed to by v.
func (c *Conn) ReadJSON(v any) error {
	_, r, err := c.NextReader()
	if err != nil {
		return err
	}
	err = json.NewDecoder(r).Decode(v)
	if err == io.EOF {
		err = io.ErrUnexpectedEOF
	}
	return err
}
