package netcmd

import (
	"log"
	"net"
	"testing"
)

func TestSimpleCall(t *testing.T) {
	s, c := net.Pipe()
	go func() {
		err := NewServer(s, nil)
		if err != nil {
			log.Println(err)
		}
	}()
	client, err := NewClient(c)
	if err != nil {
		t.Error(err)
		return
	}
	cmd := client.Command("echo", "hi")
	data, err := cmd.CombinedOutput()
	if err != nil {
		t.Error(err)
		return
	}
	log.Println("Output", string(data))
}
