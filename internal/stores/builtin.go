package stores

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

// BuiltinPrefix starts the address of an integrated store: «builtin://conad».
const BuiltinPrefix = "builtin://"

// BuiltinURL is the address an integrated store is connected with.
func BuiltinURL(id string) string { return BuiltinPrefix + id }

// IsBuiltin says whether an address is an integrated store's.
func IsBuiltin(base string) bool {
	return len(base) > len(BuiltinPrefix) && base[:len(BuiltinPrefix)] == BuiltinPrefix
}

// fetchBuiltin calls an integrated store's handler as a request would, and cuts its answer
// at the size a network one is.
func (c *Client) fetchBuiltin(ctx context.Context, id, path string, q url.Values) (status int, body []byte, err error) {
	// The handler runs in this process, on a goroutine no middleware watches: a panic in it
	// is a failed call, not a dead server.
	defer func() {
		if v := recover(); v != nil {
			status, body, err = 0, nil, fmt.Errorf("integrated store %s panicked: %v", id, v)
		}
	}()
	h, ok := c.Builtin[id]
	if !ok {
		return 0, nil, errors.New("no such integrated store")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://builtin"+path+"?"+q.Encode(), nil)
	if err != nil {
		return 0, nil, err
	}
	w := &memResponse{header: http.Header{}, code: http.StatusOK}
	h.ServeHTTP(w, req)
	if w.buf.Len() > maxBody {
		return 0, nil, errors.New("answered too much")
	}
	return w.code, w.buf.Bytes(), ctx.Err()
}

// memResponse is a response kept in memory.
type memResponse struct {
	header http.Header
	code   int
	buf    bytes.Buffer
}

func (m *memResponse) Header() http.Header  { return m.header }
func (m *memResponse) WriteHeader(code int) { m.code = code }
func (m *memResponse) Write(b []byte) (int, error) {
	if m.buf.Len()+len(b) > maxBody+1 {
		return 0, errors.New("answer too large")
	}
	return m.buf.Write(b)
}
