package netcmd

import (
	"encoding/gob"
	"io"
	"log"
	"net"
	"os/exec"

	"github.com/hashicorp/yamux"
)

func NewServer(clientConn net.Conn) error {
	server, err := yamux.Server(clientConn, nil)
	if err != nil {
		return err
	}
	var conn net.Conn
	for conn, err = server.Accept(); err == nil; {
		decoder := gob.NewDecoder(conn)
		wire := wireCmd{}
		err = decoder.Decode(&wire)

		if err != nil {

			break
		}
		cmd := exec.Command(wire.Path, wire.Args...)
		var stdinout io.ReadWriteCloser
		if wire.Stdin || wire.Stdout {
			stdinout, err = server.Accept()
			if err != nil {
				break
			}
			if wire.Stdin {
				cmd.Stdin = stdinout
			}
			if wire.Stdout {
				cmd.Stdout = stdinout
			}
		}
		if wire.Stderr {
			cmd.Stderr, err = server.Accept()
			if err != nil {
				break
			}
		}
		go func() {
			cmderr := cmd.Run()
			response := []byte{0}
			if cmderr != nil {
				response = []byte(cmderr.Error())
			}
			_, werr := conn.Write(response)
			if werr != nil {
				err = werr
				log.Println("Failed writing response to client!", err)
			}
			if stdinout != nil {
				stdinout.Close()
			}
			if cmd.Stderr != nil {
				cmd.Stderr.(io.Closer).Close()
			}
			conn.Close()
		}()
	}
	server.Close()
	return err
}
