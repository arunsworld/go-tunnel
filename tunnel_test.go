package tunnel

import (
	"net"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func port222Open() bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("", "2222"), time.Millisecond*200)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func TestSSHConnectionWithPassword(t *testing.T) {
	if !port222Open() {
		t.Skip("Port 2222 not open. Please run test_server.")
	}

	spec := &Spec{
		Host: "localhost:2222",
		User: "testuser",
		Auth: []ssh.AuthMethod{
			ssh.Password("the right password"),
		},
	}

	if err := Execute(spec); err != nil {
		t.Fatal(err)
	}
}

func TestSSHConnectionWithKey(t *testing.T) {
	if !port222Open() {
		t.Skip("Port 2222 not open. Please run test_server.")
	}

	key := PrivateKeyFile("test_server/id_rsa", "passphrase")
	if key == nil {
		t.Fatal("Could not read private key...")
	}

	spec := &Spec{
		Host: "localhost:2222",
		User: "testuser",
		Auth: []ssh.AuthMethod{
			key,
		},
	}

	if err := Execute(spec); err != nil {
		t.Fatal(err)
	}
}

func TestPortForward(t *testing.T) {
	if !port222Open() {
		t.Skip("Port 2222 not open. Please run test_server.")
	}

	spec := &Spec{
		Host: "localhost:2222",
		User: "testuser",
		Auth: []ssh.AuthMethod{
			ssh.Password("the right password"),
		},
		Forward: []Forwarder{
			Forward(1234, "localhost:2222"),
		},
	}

	if err := Execute(spec); err != nil {
		t.Fatal(err)
	}

	// First test if localhost 1234 is even open
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("", "1234"), time.Millisecond*200)
	if err != nil {
		t.Fatal("After port forward couldn't connect to port 1234")
	}
	conn.Close()

	// Now connect to it via ssh and test it works
	cspec := &Spec{
		Host: "localhost:1234",
		User: "testuser",
		Auth: []ssh.AuthMethod{
			ssh.Password("the right password"),
		},
	}

	if err := Execute(cspec); err != nil {
		t.Fatal(err)
	}
}
