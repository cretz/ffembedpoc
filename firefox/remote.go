package firefox

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
)

type remote struct {
	firefox *Firefox
	rw      io.ReadWriteCloser

	bufWrite *bufio.Writer
	sendLock sync.Mutex

	bufRead  *bufio.Reader
	recvBuf  []byte
	recvLock sync.Mutex
}

func (f *Firefox) dialRemote(addr string) (*remote, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	return &remote{
		firefox:  f,
		rw:       conn,
		bufWrite: bufio.NewWriter(conn),
		bufRead:  bufio.NewReader(conn),
		recvBuf:  make([]byte, 500),
	}, nil
}

// Should not be called concurrently
func (r *remote) send(jsonVal interface{}) error {
	r.sendLock.Lock()
	defer r.sendLock.Unlock()
	// Write len, colon, then json
	// TODO: Use buffer w/ encoder for better perf
	b, err := json.Marshal(jsonVal)
	if err != nil {
		return fmt.Errorf("failed marshaling json: %w", err)
	}
	if r.firefox.config.LogRemoteMessages {
		r.firefox.log.Debugf("Sending message: %s", b)
	}
	if _, err = r.bufWrite.WriteString(strconv.Itoa(len(b))); err != nil {
		return err
	} else if err = r.bufWrite.WriteByte(':'); err != nil {
		return err
	} else if _, err = r.bufWrite.Write(b); err != nil {
		return err
	}
	return r.bufWrite.Flush()
}

// Should not be called concurrently
func (r *remote) recv(jsonVal interface{}) error {
	r.recvLock.Lock()
	defer r.recvLock.Unlock()
	// Read until colon to get msg size
	var size int
	if sizeStr, err := r.bufRead.ReadString(':'); err != nil {
		return err
	} else if size, err = strconv.Atoi(string(sizeStr[:len(sizeStr)-1])); err != nil {
		return fmt.Errorf("invalid size string %s: %w", sizeStr[:len(sizeStr)-1], err)
	}
	// Make sure the read buf is big enough
	if len(r.recvBuf) < size {
		r.recvBuf = make([]byte, size)
	}
	// Read the rest
	if _, err := io.ReadFull(r.bufRead, r.recvBuf[:size]); err != nil {
		return err
	}
	if r.firefox.config.LogRemoteMessages {
		r.firefox.log.Debugf("Received message: %s", r.recvBuf[:size])
	}
	// Unmarshal
	if err := json.Unmarshal(r.recvBuf[:size], jsonVal); err != nil {
		return fmt.Errorf("failed unmarshaling json: %w - original string: %s", err, r.recvBuf[:size])
	}
	return nil
}
