// Package adb speaks the ADB host and sync protocols over the local server
// at 127.0.0.1:5037. Listing, stat and pull use the sync protocol directly;
// push uses the v2 send path and falls back to adb.exe when the device does
// not accept it.
package adb

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const defaultAddr = "127.0.0.1:5037"

// Client talks to one ADB server.
type Client struct {
	Addr string
}

func (c *Client) addr() string {
	if c.Addr == "" {
		return defaultAddr
	}
	return c.Addr
}

func (c *Client) dial(ctx context.Context) (net.Conn, error) {
	d := net.Dialer{Timeout: 5 * time.Second}
	return d.DialContext(ctx, "tcp", c.addr())
}

// host sends a host service request and returns the connection already past
// the OKAY status. The caller owns the connection.
func (c *Client) host(ctx context.Context, service string) (net.Conn, error) {
	conn, err := c.dial(ctx)
	if err != nil {
		return nil, fmt.Errorf("adb server at %s: %w", c.addr(), err)
	}
	if err := writeService(conn, service); err != nil {
		conn.Close()
		return nil, err
	}
	if err := readStatus(conn); err != nil {
		conn.Close()
		return nil, fmt.Errorf("%s: %w", service, err)
	}
	return conn, nil
}

func writeService(w io.Writer, service string) error {
	_, err := fmt.Fprintf(w, "%04x%s", len(service), service)
	return err
}

func readStatus(r io.Reader) error {
	buf := make([]byte, 4)
	if _, err := io.ReadFull(r, buf); err != nil {
		return err
	}
	if string(buf) == "OKAY" {
		return nil
	}
	if string(buf) != "FAIL" {
		return fmt.Errorf("unexpected status %q", buf)
	}
	n, err := readHexLen(r)
	if err != nil {
		return err
	}
	msg := make([]byte, n)
	if _, err := io.ReadFull(r, msg); err != nil {
		return err
	}
	return errors.New(string(msg))
}

func readHexLen(r io.Reader) (int, error) {
	buf := make([]byte, 4)
	if _, err := io.ReadFull(r, buf); err != nil {
		return 0, err
	}
	n, err := strconv.ParseInt(string(buf), 16, 32)
	if err != nil {
		return 0, fmt.Errorf("bad length %q", buf)
	}
	return int(n), nil
}

// Device is one line of host:devices-l.
type Device struct {
	Serial  string
	State   string // device, offline, unauthorized, ...
	Product string
	Model   string
	Device  string
}

func (d Device) Ready() bool { return d.State == "device" }

func (d Device) Label() string {
	name := strings.ReplaceAll(d.Model, "_", " ")
	if name == "" {
		name = d.Product
	}
	if name == "" {
		return d.Serial
	}
	return name
}

// Devices lists every device the server knows.
func (c *Client) Devices(ctx context.Context) ([]Device, error) {
	conn, err := c.host(ctx, "host:devices-l")
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	n, err := readHexLen(conn)
	if err != nil {
		return nil, err
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(conn, body); err != nil {
		return nil, err
	}
	return parseDevices(body), nil
}

func parseDevices(body []byte) []Device {
	var out []Device
	for _, line := range bytes.Split(body, []byte{'\n'}) {
		fields := bytes.Fields(line)
		if len(fields) < 2 {
			continue
		}
		d := Device{Serial: string(fields[0]), State: string(fields[1])}
		for _, f := range fields[2:] {
			k, v, ok := strings.Cut(string(f), ":")
			if !ok {
				continue
			}
			switch k {
			case "product":
				d.Product = v
			case "model":
				d.Model = v
			case "device":
				d.Device = v
			}
		}
		out = append(out, d)
	}
	return out
}

// Shell runs a command on the device and returns its combined output.
func (c *Client) Shell(ctx context.Context, serial, command string) (string, error) {
	conn, err := c.transport(ctx, serial)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	if err := writeService(conn, "shell:"+command); err != nil {
		return "", err
	}
	if err := readStatus(conn); err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := copyCtx(ctx, &buf, conn); err != nil && !errors.Is(err, context.Canceled) {
		return buf.String(), err
	}
	return buf.String(), ctx.Err()
}

func (c *Client) transport(ctx context.Context, serial string) (net.Conn, error) {
	service := "host:transport-any"
	if serial != "" {
		service = "host:transport:" + serial
	}
	return c.host(ctx, service)
}

// Entry is one directory entry from the sync protocol.
type Entry struct {
	Name  string
	Mode  uint32
	Size  uint32
	MTime time.Time
}

func (e Entry) IsDir() bool  { return e.Mode&0o170000 == 0o040000 }
func (e Entry) IsLink() bool { return e.Mode&0o170000 == 0o120000 }

// List lists a directory. "." and ".." are dropped.
func (c *Client) List(ctx context.Context, serial, path string) ([]Entry, error) {
	s, err := c.openSync(ctx, serial)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	if err := s.send("LIST", cString(path)); err != nil {
		return nil, err
	}
	var out []Entry
	for {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		id, body, err := s.frame()
		if err != nil {
			return out, err
		}
		switch id {
		case "DONE":
			return out, nil
		case "FAIL":
			return out, fmt.Errorf("list %s: %s", path, body)
		case "DENT":
			if len(body) < 16 {
				return out, fmt.Errorf("short directory entry")
			}
			name := string(body[16:])
			if name == "." || name == ".." || name == "" {
				continue
			}
			out = append(out, Entry{
				Name:  name,
				Mode:  binary.LittleEndian.Uint32(body[0:4]),
				Size:  binary.LittleEndian.Uint32(body[4:8]),
				MTime: time.Unix(int64(binary.LittleEndian.Uint32(body[8:12])), 0),
			})
		default:
			return out, fmt.Errorf("list %s: unexpected %q", path, id)
		}
	}
}

// Pull copies a remote file into w and returns the byte count.
// onBytes, when set, is called with the running total after each chunk.
func (c *Client) Pull(ctx context.Context, serial, path string, w io.Writer) (int64, error) {
	return c.PullProgress(ctx, serial, path, w, nil)
}

// PullProgress is Pull with a progress callback. onBytes may be nil.
func (c *Client) PullProgress(ctx context.Context, serial, path string, w io.Writer, onBytes func(int64)) (int64, error) {
	s, err := c.openSync(ctx, serial)
	if err != nil {
		return 0, err
	}
	defer s.Close()
	if err := s.send("RECV", cString(path)); err != nil {
		return 0, err
	}
	var n int64
	for {
		if err := ctx.Err(); err != nil {
			return n, err
		}
		id, body, err := s.frame()
		if err != nil {
			return n, err
		}
		switch id {
		case "DATA":
			wrote, err := w.Write(body)
			n += int64(wrote)
			if onBytes != nil {
				onBytes(n)
			}
			if err != nil {
				return n, err
			}
		case "DONE":
			return n, nil
		case "FAIL":
			return n, fmt.Errorf("pull %s: %s", path, body)
		default:
			return n, fmt.Errorf("pull %s: unexpected %q", path, id)
		}
	}
}

// Push copies a local file onto the device. It tries the sync v2 SEND
// path and falls back to adb push when the device rejects it.
func (c *Client) Push(ctx context.Context, serial, remote, local string) error {
	return c.PushProgress(ctx, serial, remote, local, nil)
}

// PushProgress is Push with a progress callback. onBytes may be nil.
// It is called with how many file bytes have been sent so far.
func (c *Client) PushProgress(ctx context.Context, serial, remote, local string, onBytes func(int64)) error {
	UploadLog("push begin local=%s remote=%s callback=%t", local, remote, onBytes != nil)
	err := c.pushSync(ctx, serial, remote, local, onBytes)
	if err == nil {
		UploadLog("push sync ok remote=%s", remote)
		return nil
	}
	if errors.Is(err, context.Canceled) {
		UploadLog("push sync canceled remote=%s", remote)
		return err
	}
	UploadLog("push sync failed remote=%s err=%v", remote, err)
	if syncWriteRejected(err) {
		return err
	}
	err = c.pushADB(ctx, serial, remote, local, onBytes)
	if err != nil {
		UploadLog("push adb failed remote=%s err=%v", remote, err)
		return err
	}
	UploadLog("push adb ok remote=%s", remote)
	return nil
}

func (c *Client) pushSync(ctx context.Context, serial, remote, local string, onBytes func(int64)) error {
	f, err := os.Open(local)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("push %s: directory", local)
	}
	s, err := c.openSync(ctx, serial)
	if err != nil {
		return err
	}
	defer s.Close()
	if err := s.send("STAT", append([]byte("v2"), 0)); err != nil {
		return err
	}
	hello := make([]byte, 16)
	if _, err := io.ReadFull(s, hello); err != nil {
		return err
	}
	if string(hello[:4]) != "STAT" {
		return fmt.Errorf("push: no sync v2 (%q)", hello[:4])
	}
	UploadLog("push sync hello remote=%s id=%q size=%d", remote, hello[:4], info.Size())
	path := remote + ",33206"
	if err := s.send("SEND", append([]byte(path), 0)); err != nil {
		return err
	}
	buf := make([]byte, 64*1024)
	var sent int64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, err := f.Read(buf)
		if n > 0 {
			if err := s.send("DATA", buf[:n]); err != nil {
				return err
			}
			sent += int64(n)
			if onBytes != nil {
				onBytes(sent)
			}
			if sent == int64(n) || sent == info.Size() || sent/1048576 != (sent-int64(n))/1048576 {
				UploadLog("push sync bytes remote=%s sent=%d size=%d", remote, sent, info.Size())
			}
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
	}
	done := make([]byte, 8)
	binary.LittleEndian.PutUint32(done, uint32(info.ModTime().Unix()))
	if err := s.send("DONE", done); err != nil {
		return err
	}
	id, body, err := s.frame()
	if err != nil {
		return err
	}
	if id != "OKAY" {
		return fmt.Errorf("push %s: %s %s", remote, id, body)
	}
	return nil
}

func (c *Client) pushADB(ctx context.Context, serial, remote, local string, onBytes func(int64)) error {
	args := []string{}
	if serial != "" {
		args = append(args, "-s", serial)
	}
	args = append(args, "push", local, remote)
	cmd := exec.CommandContext(ctx, adbExe(), args...)
	var log bytes.Buffer
	var size int64
	if info, err := os.Stat(local); err == nil {
		size = info.Size()
	}
	cmd.Stdout = &log
	cmd.Stderr = &pushProgress{size: size, onBytes: onBytes, log: &log}
	UploadLog("push adb start local=%s remote=%s size=%d", local, remote, size)
	err := cmd.Run()
	UploadLog("push adb exit err=%v output=%q", err, tailSnippet([]byte(log.String())))
	if err != nil {
		msg := strings.TrimSpace(log.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("push %s: %s", remote, msg)
	}
	if onBytes != nil && size > 0 {
		onBytes(size)
	}
	return nil
}

// pushProgress reads adb push's "45%" updates. adb rewrites the line with CR.
type pushProgress struct {
	size    int64
	onBytes func(int64)
	log     *bytes.Buffer
	last    int
	tail    []byte
}

func (p *pushProgress) Write(b []byte) (int, error) {
	if p.log != nil {
		_, _ = p.log.Write(b)
	}
	p.tail = append(p.tail, b...)
	if len(p.tail) > 256 {
		p.tail = append([]byte(nil), p.tail[len(p.tail)-256:]...)
	}
	if p.onBytes != nil && p.size > 0 {
		if pct := lastPercent(p.tail); pct > p.last {
			p.last = pct
			n := p.size * int64(pct) / 100
			UploadLog("push adb percent=%d bytes=%d size=%d tail=%q", pct, n, p.size, tailSnippet(p.tail))
			p.onBytes(n)
		}
	}
	return len(b), nil
}

// syncWriteRejected is a device refusal that a second adb push will not fix.
// Falling back replaced "No space left on device" with "exit status 1".
func syncWriteRejected(err error) bool {
	s := strings.ToLower(err.Error())
	for _, part := range []string{
		"no space left on device",
		"read-only",
		"permission denied",
		"fail write failed",
	} {
		if strings.Contains(s, part) {
			return true
		}
	}
	return false
}

func tailSnippet(b []byte) string {
	if len(b) > 180 {
		b = b[len(b)-180:]
	}
	s := string(b)
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r", "\\r"), "\n", "\\n")
}

func lastPercent(b []byte) int {
	best := -1
	for i := 0; i < len(b); i++ {
		if b[i] != '%' {
			continue
		}
		j := i - 1
		for j >= 0 && b[j] >= '0' && b[j] <= '9' {
			j--
		}
		if j == i-1 {
			continue
		}
		n, err := strconv.Atoi(string(b[j+1 : i]))
		if err != nil || n < 0 || n > 100 {
			continue
		}
		best = n
	}
	return best
}

// Install installs a local apk, replacing an existing package of the same name.
func (c *Client) Install(ctx context.Context, serial, local string) error {
	args := []string{}
	if serial != "" {
		args = append(args, "-s", serial)
	}
	args = append(args, "install", "-r", local)
	cmd := exec.CommandContext(ctx, adbExe(), args...)
	out, err := cmd.CombinedOutput()
	msg := strings.TrimSpace(string(out))
	if err != nil {
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("install: %s", msg)
	}
	if msg != "" && !strings.Contains(msg, "Success") {
		return fmt.Errorf("install: %s", msg)
	}
	return nil
}

func adbExe() string {
	if p := os.Getenv("ADB"); p != "" {
		return p
	}
	return "adb"
}

func (c *Client) openSync(ctx context.Context, serial string) (*syncConn, error) {
	return c.openSyncService(ctx, serial, "sync:")
}

func (c *Client) openSyncService(ctx context.Context, serial, service string) (*syncConn, error) {
	conn, err := c.transport(ctx, serial)
	if err != nil {
		return nil, err
	}
	if err := writeService(conn, service); err != nil {
		conn.Close()
		return nil, err
	}
	if err := readStatus(conn); err != nil {
		conn.Close()
		return nil, err
	}
	return &syncConn{Conn: conn}, nil
}

type syncConn struct{ net.Conn }

func (s *syncConn) send(id string, payload []byte) error {
	buf := make([]byte, 8+len(payload))
	copy(buf, id)
	binary.LittleEndian.PutUint32(buf[4:], uint32(len(payload)))
	copy(buf[8:], payload)
	_, err := s.Write(buf)
	return err
}

// frame reads one sync packet. DONE's 4-byte trailer is discarded; DENT
// returns mode, size, mtime, namelen and the name.
func (s *syncConn) frame() (string, []byte, error) {
	hdr := make([]byte, 4)
	if _, err := io.ReadFull(s, hdr); err != nil {
		return "", nil, err
	}
	id := string(hdr)
	switch id {
	case "DONE":
		tail := make([]byte, 4)
		_, err := io.ReadFull(s, tail)
		return id, tail, err
	case "OKAY":
		return id, nil, nil
	case "DENT":
		rest := make([]byte, 16)
		if _, err := io.ReadFull(s, rest); err != nil {
			return "", nil, err
		}
		n := binary.LittleEndian.Uint32(rest[12:16])
		if n > 16<<20 {
			return "", nil, fmt.Errorf("directory name of %d bytes", n)
		}
		name := make([]byte, n)
		if _, err := io.ReadFull(s, name); err != nil {
			return "", nil, err
		}
		return id, append(rest, name...), nil
	default:
		lenBuf := make([]byte, 4)
		if _, err := io.ReadFull(s, lenBuf); err != nil {
			return "", nil, err
		}
		n := binary.LittleEndian.Uint32(lenBuf)
		if n > 64<<20 {
			return "", nil, fmt.Errorf("%s payload of %d bytes", id, n)
		}
		body := make([]byte, n)
		if _, err := io.ReadFull(s, body); err != nil {
			return "", nil, err
		}
		return id, body, nil
	}
}

func cString(s string) []byte { return append([]byte(s), 0) }

func copyCtx(ctx context.Context, dst io.Writer, src net.Conn) error {
	buf := make([]byte, 32*1024)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		_ = src.SetReadDeadline(time.Now().Add(250 * time.Millisecond))
		n, err := src.Read(buf)
		if n > 0 {
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return werr
			}
		}
		if err == nil {
			continue
		}
		if ne, ok := err.(net.Error); ok && ne.Timeout() {
			continue
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
}
