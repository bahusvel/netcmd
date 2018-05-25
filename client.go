package netcmd

import (
	"bytes"
	"encoding/gob"
	"errors"
	"io"
	"io/ioutil"
	"net"
	"sync"

	"github.com/hashicorp/yamux"
)

type Client struct {
	cmdMutex   sync.Mutex
	serverConn net.Conn
	session    *yamux.Session
}

type Cmd struct {
	client *Client
	Path   string
	Args   []string

	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	status      net.Conn
	netstdinout io.ReadWriteCloser
	netstderr   io.ReadCloser
}

type wireMode int

const (
	tunnelMode wireMode = iota
	cmdMode             = iota
)

type wireCmd struct {
	Path string
	Args []string

	Stdin  bool
	Stdout bool
	Stderr bool
}

type singleWriter struct {
	b  bytes.Buffer
	mu sync.Mutex
}

func (w *singleWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.Write(p)
}

func NewClient(serverConn net.Conn) (client *Client, err error) {
	client = &Client{serverConn: serverConn}
	client.session, err = yamux.Client(serverConn, nil)
	if err != nil {
		return
	}
	return
}

func (this *Client) Command(name string, arg ...string) *Cmd {
	return &Cmd{Path: name, Args: arg, client: this}
}

func (this *Cmd) CombinedOutput() (data []byte, err error) {
	buffer := singleWriter{}
	this.Stdout = &buffer
	this.Stderr = &buffer
	err = this.Run()
	data = buffer.b.Bytes()
	return
}

func (this *Cmd) Output() (data []byte, err error) {
	this.Stdout = &bytes.Buffer{}
	err = this.Run()
	data = this.Stdout.(*bytes.Buffer).Bytes()
	return
}

func (this *Cmd) Run() (err error) {
	err = this.exec()
	if err != nil {
		return
	}
	return this.wait()
}

func (this *Cmd) wait() (err error) {
	var ret []byte
	ret, err = ioutil.ReadAll(this.status)
	if err != nil {
		return
	}
	if len(ret) != 1 && ret[0] != 0 {
		err = errors.New(string(ret))
	}
	if this.netstderr != nil {
		this.netstderr.Close()
	}
	if this.netstdinout != nil {
		this.netstdinout.Close()
	}
	this.status.Close()
	return
}

func (this *Cmd) Start() (err error) {
	err = this.exec()
	if err != nil {
		return
	}
	go this.wait()
	return
}

func (this *Client) OpenTunnel() (net.Conn, error) {
	this.cmdMutex.Lock()
	defer this.cmdMutex.Unlock()

	session, err := this.session.Open()
	if err != nil {
		return nil, err
	}
	encoder := gob.NewEncoder(session)

	err = encoder.Encode(tunnelMode)
	if err != nil {
		session.Close()
		return nil, err
	}

	return session, nil
}

func (this *Cmd) exec() (err error) {
	this.client.cmdMutex.Lock()
	defer this.client.cmdMutex.Unlock()

	this.status, err = this.client.session.Open()
	if err != nil {
		return
	}
	encoder := gob.NewEncoder(this.status)

	wire := wireCmd{
		Path:   this.Path,
		Args:   this.Args,
		Stdin:  this.Stdin != nil,
		Stdout: this.Stdout != nil,
		Stderr: this.Stderr != nil,
	}

	err = encoder.Encode(cmdMode)
	if err != nil {
		this.status.Close()
		return
	}

	err = encoder.Encode(wire)
	if err != nil {
		this.status.Close()
		return
	}

	if this.Stdin != nil || this.Stdout != nil {
		this.netstdinout, err = this.client.session.Open()
		if err != nil {
			this.status.Close()
			return
		}
	}
	if this.Stdin != nil {
		go io.Copy(this.netstdinout, this.Stdin)
	}
	if this.Stdout != nil {
		go io.Copy(this.Stdout, this.netstdinout)
	}
	if this.Stderr != nil {
		this.netstderr, err = this.client.session.Open()
		if err != nil {
			this.status.Close()
			return
		}
		go io.Copy(this.Stderr, this.netstderr)
	}
	return nil
}
