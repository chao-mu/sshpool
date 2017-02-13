package sshpool

import (
	"errors"
	"net"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

var (
	user         = "testuser"
	password     = "foo"
	serverConfig = &ssh.ServerConfig{
		PasswordCallback: func(conn ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if conn.User() == user && string(pass) == password {
				return nil, nil
			}
			return nil, errors.New("invalid credentials")
		},
	}
	clientConfig = &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.Password(password),
		},
	}
)

func init() {
	signer, err := ssh.ParsePrivateKey([]byte(testServerPrivateKey))
	if err != nil {
		panic("unable to set private key: " + err.Error())
	}
	serverConfig.AddHostKey(signer)
}

type serverBehavior struct {
	sessionDelay time.Duration
}

func dial(t *testing.T) net.Conn {
	return configDial(t, new(serverBehavior))
}

func configDial(t *testing.T, b *serverBehavior) net.Conn {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal("unable to listen:", err)
	}
	go func() {
		defer l.Close()
		nConn, err := l.Accept()
		if err != nil {
			t.Error("unable to accept:", err)
			return
		}
		defer nConn.Close()
		_, chans, reqs, err := ssh.NewServerConn(nConn, serverConfig)
		if err != nil {
			t.Error("unable to handshake:", err)
			return
		}
		go ssh.DiscardRequests(reqs)
		for newChannel := range chans {
			time.Sleep(b.sessionDelay)
			ch, _, err := newChannel.Accept()
			if err != nil {
				t.Error("unable to accept:", err)
				return
			}
			ch.Close()
		}
	}()
	c, err := net.Dial("tcp", l.Addr().String())
	if err != nil {
		t.Fatal("unable to dial test server:", err)
	}
	return c
}

func TestOpenReuse(t *testing.T) {
	c := 0
	p := &Pool{Dial: func(net, addr string) (net.Conn, error) {
		c++
		return dial(t), nil
	}}
	_, err := p.Open("net", "addr", clientConfig)
	if err != nil {
		t.Fatal("unexpected error:", err)
	}
	_, err = p.Open("net", "addr", clientConfig)
	if err != nil {
		t.Fatal("unexpected error:", err)
	}
	if c != 1 {
		t.Fatalf("want 1 call, got %d calls", c)
	}
}

func TestSessionTimeout(t *testing.T) {
	p := &Pool{Dial: func(net, addr string) (net.Conn, error) {
		return configDial(t, &serverBehavior{sessionDelay: (5 * time.Second)}), nil
	}, Timeout: (100 * time.Millisecond)}
	_, err := p.Open("net", "addr", clientConfig)
	if err == nil {
		t.Fatal("expected timeout error; got nil")
	}
}

func TestOpenDistinct(t *testing.T) {
	c := 0
	p := &Pool{Dial: func(net, addr string) (net.Conn, error) {
		c++
		return dial(t), nil
	}}
	_, err := p.Open("net", "addr0", clientConfig)
	if err != nil {
		t.Fatal("unexpected error:", err)
	}
	_, err = p.Open("net", "addr1", clientConfig)
	if err != nil {
		t.Fatal("unexpected error:", err)
	}
	if c != 2 {
		t.Fatal("want 1 call, got %d calls", c)
	}
}

func TestOpenFirstError(t *testing.T) {
	p := &Pool{Dial: func(net, addr string) (net.Conn, error) {
		return nil, errors.New("test error")
	}}
	_, err := p.Open("net", "addr0", clientConfig)
	if err == nil {
		t.Fatal("expected error")
	}
}

type failReader bool

func (r *failReader) Read(p []byte) (int, error) {
	if r != nil && *r {
		return 0, errors.New("fail")
	}
	return len(p), nil
}

func TestOpenRetry(t *testing.T) {
	c := 0
	rand := new(failReader)
	p := &Pool{Dial: func(net, addr string) (net.Conn, error) {
		c++
		conn := dial(t)
		return conn, nil
	}}
	config := new(ssh.ClientConfig)
	*config = *clientConfig
	config.Rand = rand
	_, err := p.Open("net", "addr", config)
	if err != nil {
		t.Fatal("unexpected error:", err)
	}
	conn := p.tab[p.key("net", "addr", config)].c
	*rand = true
	config.Rand = nil
	_, err = p.Open("net", "addr", config)
	if err != nil {
		t.Fatal("unexpected error:", err)
	}
	if c != 2 {
		t.Fatalf("calls = %d want 2", c)
	}
	const errClosing = "use of closed network connection" // from package net
	if err := conn.Close(); err == nil || err.Error() != errClosing {
		t.Fatalf("conn still open, want closed; err = %v", err)
	}
}

func TestOpenSecondError(t *testing.T) {
	var conn net.Conn
	p := &Pool{Dial: func(net, addr string) (net.Conn, error) {
		if conn != nil {
			return nil, errors.New("test error")
		}
		conn = dial(t)
		return conn, nil
	}}
	_, err := p.Open("net", "addr", clientConfig)
	if err != nil {
		t.Fatal("unexpected error:", err)
	}
	conn.Close()
	_, err = p.Open("net", "addr", clientConfig)
	if err == nil {
		t.Fatal("expected error")
	}
}

// private key for mock server
const testServerPrivateKey = `-----BEGIN RSA PRIVATE KEY-----
MIIEpAIBAAKCAQEA19lGVsTqIT5iiNYRgnoY1CwkbETW5cq+Rzk5v/kTlf31XpSU
70HVWkbTERECjaYdXM2gGcbb+sxpq6GtXf1M3kVomycqhxwhPv4Cr6Xp4WT/jkFx
9z+FFzpeodGJWjOH6L2H5uX1Cvr9EDdQp9t9/J32/qBFntY8GwoUI/y/1MSTmMiF
tupdMODN064vd3gyMKTwrlQ8tZM6aYuyOPsutLlUY7M5x5FwMDYvnPDSeyT/Iw0z
s3B+NCyqeeMd2T7YzQFnRATj0M7rM5LoSs7DVqVriOEABssFyLj31PboaoLhOKgc
qoM9khkNzr7FHVvi+DhYM2jD0DwvqZLN6NmnLwIDAQABAoIBAQCGVj+kuSFOV1lT
+IclQYA6bM6uY5mroqcSBNegVxCNhWU03BxlW//BE9tA/+kq53vWylMeN9mpGZea
riEMIh25KFGWXqXlOOioH8bkMsqA8S7sBmc7jljyv+0toQ9vCCtJ+sueNPhxQQxH
D2YvUjfzBQ04I9+wn30BByDJ1QA/FoPsunxIOUCcRBE/7jxuLYcpR+JvEF68yYIh
atXRld4W4in7T65YDR8jK1Uj9XAcNeDYNpT/M6oFLx1aPIlkG86aCWRO19S1jLPT
b1ZAKHHxPMCVkSYW0RqvIgLXQOR62D0Zne6/2wtzJkk5UCjkSQ2z7ZzJpMkWgDgN
ifCULFPBAoGBAPoMZ5q1w+zB+knXUD33n1J+niN6TZHJulpf2w5zsW+m2K6Zn62M
MXndXlVAHtk6p02q9kxHdgov34Uo8VpuNjbS1+abGFTI8NZgFo+bsDxJdItemwC4
KJ7L1iz39hRN/ZylMRLz5uTYRGddCkeIHhiG2h7zohH/MaYzUacXEEy3AoGBANz8
e/msleB+iXC0cXKwds26N4hyMdAFE5qAqJXvV3S2W8JZnmU+sS7vPAWMYPlERPk1
D8Q2eXqdPIkAWBhrx4RxD7rNc5qFNcQWEhCIxC9fccluH1y5g2M+4jpMX2CT8Uv+
3z+NoJ5uDTXZTnLCfoZzgZ4nCZVZ+6iU5U1+YXFJAoGBANLPpIV920n/nJmmquMj
orI1R/QXR9Cy56cMC65agezlGOfTYxk5Cfl5Ve+/2IJCfgzwJyjWUsFx7RviEeGw
64o7JoUom1HX+5xxdHPsyZ96OoTJ5RqtKKoApnhRMamau0fWydH1yeOEJd+TRHhc
XStGfhz8QNa1dVFvENczja1vAoGABGWhsd4VPVpHMc7lUvrf4kgKQtTC2PjA4xoc
QJ96hf/642sVE76jl+N6tkGMzGjnVm4P2j+bOy1VvwQavKGoXqJBRd5Apppv727g
/SM7hBXKFc/zH80xKBBgP/i1DR7kdjakCoeu4ngeGywvu2jTS6mQsqzkK+yWbUxJ
I7mYBsECgYB/KNXlTEpXtz/kwWCHFSYA8U74l7zZbVD8ul0e56JDK+lLcJ0tJffk
gqnBycHj6AhEycjda75cs+0zybZvN4x65KZHOGW/O/7OAWEcZP5TPb3zf9ned3Hl
NsZoFj52ponUM6+99A2CmezFCN16c4mbA//luWF+k3VVqR6BpkrhKw==
-----END RSA PRIVATE KEY-----`
