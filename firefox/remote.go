package firefox

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strconv"
)

type remote struct {
	rw       io.ReadWriteCloser
	bufWrite *bufio.Writer
	bufRead  *bufio.Reader
	recvBuf  []byte
}

func dialRemote(addr string) (*remote, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	return &remote{
		rw:       conn,
		bufWrite: bufio.NewWriter(conn),
		bufRead:  bufio.NewReader(conn),
		recvBuf:  make([]byte, 500),
	}, nil
}

// Should not be called concurrently
func (r *remote) send(v map[string]interface{}) error {
	// Write len, colon, then json
	// TODO: Use buffer w/ encoder for better perf
	if b, err := json.Marshal(v); err != nil {
		return fmt.Errorf("failed marshaling json: %w", err)
	} else if _, err = r.bufWrite.WriteString(strconv.Itoa(len(b))); err != nil {
		return err
	} else if err = r.bufWrite.WriteByte(':'); err != nil {
		return err
	} else if _, err = r.bufWrite.Write(b); err != nil {
		return err
	}
	return r.bufWrite.Flush()
}

// Should not be called concurently
func (r *remote) recv() (map[string]interface{}, error) {
	// Read until colon to get msg size
	var size int
	if sizeStr, err := r.bufRead.ReadString(':'); err != nil {
		return nil, err
	} else if size, err = strconv.Atoi(string(sizeStr[:len(sizeStr)-1])); err != nil {
		return nil, fmt.Errorf("invalid size string %s: %w", sizeStr[:len(sizeStr)-1], err)
	}
	// Make sure the read buf is big enough
	if len(r.recvBuf) < size {
		r.recvBuf = make([]byte, size)
	}
	// Read the rest
	if _, err := io.ReadFull(r.bufRead, r.recvBuf[:size]); err != nil {
		return nil, err
	}
	// Unmarshal
	var ret map[string]interface{}
	if err := json.Unmarshal(r.recvBuf[:size], &ret); err != nil {
		return nil, fmt.Errorf("failed unmarshaling json: %w - original string: %s", err, r.recvBuf[:size])
	}
	return ret, nil
}
