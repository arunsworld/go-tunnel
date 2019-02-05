package tunnel

import (
	"errors"
	"io"
	"io/ioutil"
	"log"
	"net"
	"strconv"
	"time"

	"golang.org/x/crypto/ssh"
)

// Spec defines the ssh tunnel specifications
type Spec struct {
	Host           string
	User           string
	Auth           []ssh.AuthMethod
	Forward        []Forwarder
	Logger         Logger
	ForwardTimeout time.Duration
	Die            chan struct{}
}

// Forwarder defines a port forward definition
type Forwarder struct {
	port        int
	destination string
}

// Logger performs logging
type Logger interface {
	Log(format string, v ...interface{})
}

type emptyLogger struct{}

func (l *emptyLogger) Log(format string, v ...interface{}) {}

type stdoutLogger struct{}

func (l *stdoutLogger) Log(format string, v ...interface{}) {
	log.Printf(format, v...)
}

// EmptyLogger returns a logger that does nothing
func EmptyLogger() Logger {
	return &emptyLogger{}
}

// StdOutLogger returns a logger that prints to stdout
func StdOutLogger() Logger {
	return &stdoutLogger{}
}

// Execute executes the ssh connection & creation of the required tunnel
func Execute(spec *Spec) error {
	if spec.Logger == nil {
		spec.Logger = EmptyLogger()
	}
	config := getSSHConfig(spec)
	serverConnection, err := makeServerConnection(spec, config)
	if err != nil {
		return err
	}
	timeout := time.Second * 5
	if spec.ForwardTimeout > 0 {
		timeout = spec.ForwardTimeout
	}
	for _, f := range spec.Forward {
		if !isDestinationAvailable(serverConnection, f.destination, timeout) {
			spec.Logger.Log("%s is not available. Not bothering with tunnel.", f.destination)
			serverConnection.Close()
			return errors.New("destination not available... closing down")
		}
		localListener := listenLocally(f.port, spec.Logger)
		if localListener == nil {
			serverConnection.Close()
			return errors.New("could not open local port... closing down")
		}
		go acceptNewConnectionAndTunnel(localListener, serverConnection, f, spec.Logger)
	}
	go monitorOrDie(serverConnection, spec)
	return nil
}

func monitorOrDie(serverConnection *ssh.Client, spec *Spec) {
	timeout := time.Second * 5
	if spec.ForwardTimeout > 0 {
		timeout = spec.ForwardTimeout
	}
	for {
		for _, f := range spec.Forward {
			if !isDestinationAvailable(serverConnection, f.destination, timeout) {
				spec.Logger.Log("%s is unreachable.", f.destination)
				// serverConnection.Close()
				// if spec.Die != nil {
				// 	spec.Die <- struct{}{}
				// }
				// return
			}
		}
		// spec.Logger.Log("All good within connection: %s", spec.Host)
		time.Sleep(time.Second * 10)
	}
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

func listenLocally(port int, logger Logger) net.Listener {
	conn, err := net.Listen("tcp", "localhost:"+strconv.Itoa(port))
	if err != nil {
		logger.Log("Unable to bind to local port: %d\n", port)
		return nil
	}
	return conn
}

func isDestinationAvailable(serverConnection *ssh.Client, destination string, timeout time.Duration) bool {
	done := make(chan bool)
	go func() {
		conn, err := serverConnection.Dial("tcp", destination)
		if err != nil {
			done <- false
			return
		}
		conn.Close()
		done <- true
	}()

	select {
	case status := <-done:
		return status
	case <-time.After(timeout):
		return false
	}
}

func acceptNewConnectionAndTunnel(localListener net.Listener, serverConnection *ssh.Client, forwarder Forwarder, logger Logger) {
	defer localListener.Close()

	for {
		conn, err := localListener.Accept()
		if err != nil {
			logger.Log("Could not accept new connection on port %d: %s\n", forwarder.port, err.Error())
			return
		}
		logger.Log("Connection accepted on port: %d\n", forwarder.port)
		go tunnel(serverConnection, conn, forwarder.destination, logger)
	}
}

func tunnel(serverConnection *ssh.Client, localConnection net.Conn, destination string, logger Logger) {
	remoteConnection, err := serverConnection.Dial("tcp", destination)
	if err != nil {
		logger.Log("Unable to connect to remote destination %s: %s\n", destination, err.Error())
		localConnection.Close()
		return
	}

	copyConn := func(writer, reader net.Conn) {
		_, err := io.Copy(writer, reader)
		if err != nil {
			logger.Log("io.Copy error:", err)
		}
	}

	go copyConn(localConnection, remoteConnection)
	go copyConn(remoteConnection, localConnection)
}

// PrivateKeyFile reads a private key and returns an AuthMethod using it
func PrivateKeyFile(file string, passPhrase string) (ssh.AuthMethod, error) {
	buffer, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, errors.New("Couldn't read private key:" + err.Error())
	}

	key, err := ssh.ParsePrivateKeyWithPassphrase(buffer, []byte(passPhrase))
	if err != nil {
		return nil, errors.New("Couldn't parse private key:" + err.Error())
	}

	return ssh.PublicKeys(key), nil
}
