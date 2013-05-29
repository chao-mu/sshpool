package sshpool

import (
	"code.google.com/p/go.crypto/ssh"
	"errors"
	"io"
	"testing"
)

// password implements ssh.ClientPassword
type password string

func (p password) Password(user string) (string, error) {
	return string(p), nil
}

var (
	clientPassword = password("foo")
	serverConfig   = &ssh.ServerConfig{
		PasswordCallback: func(conn *ssh.ServerConn, user, pass string) bool {
			return user == "testuser" && pass == string(clientPassword)
		},
		PublicKeyCallback: func(conn *ssh.ServerConn, user, algo string, pubkey []byte) bool {
			return false
		},
	}
)

func init() {
	if err := serverConfig.SetRSAPrivateKey([]byte(testServerPrivateKey)); err != nil {
		panic("unable to set private key: " + err.Error())
	}
}

func dial(t *testing.T) *ssh.ClientConn {
	l, err := ssh.Listen("tcp", "127.0.0.1:0", serverConfig)
	if err != nil {
		t.Fatal("unable to listen:", err)
	}
	go func() {
		defer l.Close()
		conn, err := l.Accept()
		if err != nil {
			t.Error("unable to accept:", err)
			return
		}
		defer conn.Close()
		if err := conn.Handshake(); err != nil {
			t.Error("unable to handshake:", err)
			return
		}
		for {
			ch, err := conn.Accept()
			if err == io.EOF {
				return
			}
			if err != nil {
				t.Error("unable to accept:", err)
				return
			}
			ch.Accept()
			ch.Close()
		}
	}()
	config := &ssh.ClientConfig{
		User: "testuser",
		Auth: []ssh.ClientAuth{
			ssh.ClientAuthPassword(clientPassword),
		},
	}
	c, err := ssh.Dial("tcp", l.Addr().String(), config)
	if err != nil {
		t.Fatal("unable to dial test server:", err)
	}
	return c
}

func TestOpenReuse(t *testing.T) {
	c := 0
	p := &Pool{Dial: func(net, addr string, config *ssh.ClientConfig) (*ssh.ClientConn, error) {
		c++
		return dial(t), nil
	}}
	config := &ssh.ClientConfig{
		User: "u",
	}
	_, err := p.Open("net", "addr", config)
	if err != nil {
		t.Fatal("unexpected error:", err)
	}
	_, err = p.Open("net", "addr", config)
	if err != nil {
		t.Fatal("unexpected error:", err)
	}
	if c != 1 {
		t.Fatalf("want 1 call, got %d calls", c)
	}
}

func TestOpenDistinct(t *testing.T) {
	c := 0
	p := &Pool{Dial: func(net, addr string, config *ssh.ClientConfig) (*ssh.ClientConn, error) {
		c++
		return dial(t), nil
	}}
	config := &ssh.ClientConfig{
		User: "u",
	}
	_, err := p.Open("net", "addr0", config)
	if err != nil {
		t.Fatal("unexpected error:", err)
	}
	_, err = p.Open("net", "addr1", config)
	if err != nil {
		t.Fatal("unexpected error:", err)
	}
	if c != 2 {
		t.Fatal("want 1 call, got %d calls", c)
	}
}

func TestOpenFirstError(t *testing.T) {
	p := &Pool{Dial: func(net, addr string, config *ssh.ClientConfig) (*ssh.ClientConn, error) {
		return nil, errors.New("test error")
	}}
	config := &ssh.ClientConfig{
		User: "u",
	}
	_, err := p.Open("net", "addr0", config)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestOpenRetry(t *testing.T) {
	c := 0
	var lastConn *ssh.ClientConn
	p := &Pool{Dial: func(net, addr string, config *ssh.ClientConfig) (*ssh.ClientConn, error) {
		c++
		conn := dial(t)
		lastConn = conn
		return conn, nil
	}}
	config := &ssh.ClientConfig{
		User: "u",
	}
	_, err := p.Open("net", "addr", config)
	if err != nil {
		t.Fatal("unexpected error:", err)
	}
	lastConn.Close()
	_, err = p.Open("net", "addr", config)
	if err != nil {
		t.Fatal("unexpected error:", err)
	}
	if c != 2 {
		t.Fatal("want 1 call, got %d calls", c)
	}
}

func TestOpenSecondError(t *testing.T) {
	var conn *ssh.ClientConn
	p := &Pool{Dial: func(net, addr string, config *ssh.ClientConfig) (*ssh.ClientConn, error) {
		if conn != nil {
			return nil, errors.New("test error")
		}
		conn = dial(t)
		return conn, nil
	}}
	config := &ssh.ClientConfig{
		User: "u",
	}
	_, err := p.Open("net", "addr", config)
	if err != nil {
		t.Fatal("unexpected error:", err)
	}
	conn.Close()
	_, err = p.Open("net", "addr", config)
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