package tunnel

import (
	"io"
	"io/ioutil"
	"log"
	"net"
	"strconv"

	"golang.org/x/crypto/ssh"
)

// Spec defines the ssh tunnel specifications
type Spec struct {
	Host    string
	User    string
	Auth    []ssh.AuthMethod
	Forward []Forwarder
}

// Forwarder defines a port forward definition
type Forwarder struct {
	port        int
	destination string
}

// Execute executes the ssh connection & creation of the required tunnel
func Execute(spec *Spec) error {
	config := getSSHConfig(spec)
	serverConnection, err := makeServerConnection(spec, config)
	if err != nil {
		return err
	}
	for _, f := range spec.Forward {
		localConnection := listenLocally(f.port)
		if localConnection == nil {
			continue
		}
		go forwardConnection(localConnection, serverConnection, f)
	}
	return nil
}

func getSSHConfig(spec *Spec) *ssh.ClientConfig {
	return &ssh.ClientConfig{
		User: spec.User,
		Auth: spec.Auth,
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			return nil
		},
	}
}

func makeServerConnection(spec *Spec, config *ssh.ClientConfig) (*ssh.Client, error) {
	return ssh.Dial("tcp", spec.Host, config)
}

// Forward returns a Forwarder based on input param
func Forward(port int, destination string) Forwarder {
	return Forwarder{
		port:        port,
		destination: destination,
	}
}

func listenLocally(port int) net.Listener {
	conn, err := net.Listen("tcp", "localhost:"+strconv.Itoa(port))
	if err != nil {
		log.Printf("Unable to bind to local port: %d\n", port)
		return nil
	}
	return conn
}

func forwardConnection(localConnection net.Listener, serverConnection *ssh.Client, forwarder Forwarder) {
	defer localConnection.Close()

	for {
		conn, err := localConnection.Accept()
		if err != nil {
			log.Printf("Could not accept new connection on port %d: %s\n", forwarder.port, err.Error())
			return
		}
		log.Printf("Connection accepted on port: %d\n", forwarder.port)
		tunnel(serverConnection, conn, forwarder.destination)
	}
}

func tunnel(serverConnection *ssh.Client, localConnection net.Conn, destination string) {
	remoteConnection, err := serverConnection.Dial("tcp", destination)
	if err != nil {
		log.Printf("Unable to connect to remote destination %s: %s\n", destination, err.Error())
		return
	}

	copyConn := func(writer, reader net.Conn) {
		_, err := io.Copy(writer, reader)
		if err != nil {
			log.Println("io.Copy error:", err)
		}
	}

	go copyConn(localConnection, remoteConnection)
	go copyConn(remoteConnection, localConnection)
}

// PrivateKeyFile reads a private key and returns an AuthMethod using it
func PrivateKeyFile(file string, passPhrase string) ssh.AuthMethod {
	buffer, err := ioutil.ReadFile(file)
	if err != nil {
		log.Println("Couldn't read private key:", err)
		return nil
	}

	key, err := ssh.ParsePrivateKeyWithPassphrase(buffer, []byte(passPhrase))
	if err != nil {
		log.Println("Couldn't parse private key:", err)
		return nil
	}

	return ssh.PublicKeys(key)
}
