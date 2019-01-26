package tunnel

import (
	"fmt"
	"log"
	"net"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func port2229Open() bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("", "2229"), time.Millisecond*200)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func TestSSHConnectionWithPassword(t *testing.T) {
	if !port2229Open() {
		t.Fatal("Port 2229 not open. Please run test_server.")
	}

	spec := &Spec{
		Host: "localhost:2229",
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
	if !port2229Open() {
		t.Fatal("Port 2229 not open. Please run test_server.")
	}

	key, err := PrivateKeyFile("test_server/id_rsa", "passphrase")
	if err != nil {
		t.Fatal(err)
	}

	spec := &Spec{
		Host: "localhost:2229",
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
	if !port2229Open() {
		t.Fatal("Port 2229 not open. Please run test_server.")
	}

	spec := &Spec{
		Host: "localhost:2229",
		User: "testuser",
		Auth: []ssh.AuthMethod{
			ssh.Password("the right password"),
		},
		Forward: []Forwarder{
			Forward(1234, "localhost:2229"),
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

func TestErrorCases(t *testing.T) {
	if !port2229Open() {
		t.Fatal("Port 2229 not open. Please run test_server.")
	}

	t.Run("Bad Spec", func(t *testing.T) {
		spec := &Spec{}
		err := Execute(spec)
		if err == nil {
			t.Fatal("Expected an error but didn't get it!")
		}
	})

	t.Run("Bad Local Port during Forward", func(t *testing.T) {
		spec := &Spec{
			Host: "localhost:2229",
			User: "testuser",
			Auth: []ssh.AuthMethod{
				ssh.Password("the right password"),
			},
			Forward: []Forwarder{
				Forward(222922, "localhost:2223"),
			},
		}

		if err := Execute(spec); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("Bad Destination during Forward", func(t *testing.T) {
		spec := &Spec{
			Host: "localhost:2229",
			User: "testuser",
			Auth: []ssh.AuthMethod{
				ssh.Password("the right password"),
			},
			Forward: []Forwarder{
				Forward(1235, "localhost:8989"),
			},
		}

		if err := Execute(spec); err != nil {
			t.Fatal(err)
		}

		_, err := net.DialTimeout("tcp", net.JoinHostPort("", "1235"), time.Millisecond*200)
		if err == nil {
			t.Fatal("Expected not to be able to connect to 1235 port but did!")
		}
	})

	t.Run("IO Copy Error", func(t *testing.T) {
		t.Skip("Unable to test IO Copy Error")
		//Run a service on port 6767
		conn, err := net.Listen("tcp", "localhost:6767")
		if err != nil {
			t.Fatal("Unable to listen on 6767 for this test...")
		}
		go func() {
			for {
				x, err := conn.Accept()
				if err != nil {
					log.Println("6767 broken...")
					return
				}
				fmt.Println("Connection accepted on 6767...")
				x.Write([]byte("test"))
				fmt.Println("wrote test...")
				conn.Close()
			}
		}()

		// Now portforward to port 6767
		spec := &Spec{
			Host: "localhost:2229",
			User: "testuser",
			Auth: []ssh.AuthMethod{
				ssh.Password("the right password"),
			},
			Forward: []Forwarder{
				Forward(6768, "localhost:6767"),
			},
		}

		if err := Execute(spec); err != nil {
			t.Fatal(err)
		}

		// Read from post 6768 then close 6767 to create a copy exception
		cc, err := net.DialTimeout("tcp", net.JoinHostPort("", "6768"), time.Millisecond*200)
		if err != nil {
			t.Fatal("Could not connect to port 6768")
		}
		b := make([]byte, 10)
		cc.Read(b)
		fmt.Println(string(b))
		cc.Write([]byte("great"))
		fmt.Println("wrote great...")
	})
}

func TestBadPrivateKey(t *testing.T) {

	t.Run("File Does Not Exist", func(t *testing.T) {
		_, err := PrivateKeyFile("/tmp/doesnotexist.xxx", "")
		if err == nil {
			t.Fatal("Expecting an error...")
		}
	})

	t.Run("File Is Not a Private Key", func(t *testing.T) {
		_, err := PrivateKeyFile("tunnel_test.go", "")
		if err == nil {
			t.Fatal("Expecting an error...")
		}
	})

}
