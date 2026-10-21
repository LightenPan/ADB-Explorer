package adb

import (
	"encoding/binary"
	"net"
	"testing"
)

func TestParseDevices(t *testing.T) {
	body := []byte("0123456789ABCDEF       device product:Pixel model:Pixel_7 device:panther transport_id:1\n" +
		"emulator-5554          offline transport_id:2\n")
	got := parseDevices(body)
	if len(got) != 2 {
		t.Fatalf("got %d devices", len(got))
	}
	if got[0].Serial != "0123456789ABCDEF" || got[0].Model != "Pixel_7" || !got[0].Ready() {
		t.Fatalf("device 0: %+v", got[0])
	}
	if got[0].Label() != "Pixel 7" {
		t.Fatalf("label %q", got[0].Label())
	}
	if got[1].Ready() {
		t.Fatal("offline device reported ready")
	}
}

func TestListFraming(t *testing.T) {
	srv, cli := net.Pipe()
	defer srv.Close()
	defer cli.Close()
	go func() {
		srv.Write([]byte("OKAY"))
		var payload []byte
		payload = append(payload, dent("Download", 0o40770, 12, 1_700_000_000)...)
		payload = append(payload, dent(".", 0o40755, 0, 0)...)
		payload = append(payload, []byte("DONE")...)
		payload = append(payload, 0, 0, 0, 0)
		srv.Write(payload)
	}()

	s := &syncConn{Conn: cli}
	if err := readStatus(cli); err != nil {
		t.Fatal(err)
	}
	id, body, err := s.frame()
	if err != nil || id != "DENT" || string(body[16:]) != "Download" {
		t.Fatalf("dent %q %q %v", id, body, err)
	}
	mode := binary.LittleEndian.Uint32(body[0:4])
	if e := (Entry{Mode: mode}); !e.IsDir() {
		t.Fatal("expected a directory")
	}
	id, _, err = s.frame()
	if err != nil || id != "DENT" {
		t.Fatalf("second %q %v", id, err)
	}
	id, _, err = s.frame()
	if err != nil || id != "DONE" {
		t.Fatalf("done %q %v", id, err)
	}
}

func dent(name string, mode, size, mtime uint32) []byte {
	buf := make([]byte, 20+len(name))
	copy(buf, "DENT")
	binary.LittleEndian.PutUint32(buf[4:], mode)
	binary.LittleEndian.PutUint32(buf[8:], size)
	binary.LittleEndian.PutUint32(buf[12:], mtime)
	binary.LittleEndian.PutUint32(buf[16:], uint32(len(name)))
	copy(buf[20:], name)
	return buf
}
