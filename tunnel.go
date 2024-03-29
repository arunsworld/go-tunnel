package tunnel

import (
	"context"
	"errors"
	"io"
	"io/ioutil"
	"log"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/arunsworld/nursery"
	"golang.org/x/crypto/ssh"
)

// Spec defines the ssh tunnel specifications
type Spec struct {
	Host           string
	User           string
	Auth           []ssh.AuthMethod
	Forward        []Forwarder
	Reverse        []Forwarder
	Logger         Logger
	ForwardTimeout time.Duration
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
	localConnection := localNetwork{}
	if spec.ForwardTimeout == 0 {
		spec.ForwardTimeout = time.Second * 5
	}
	for _, f := range spec.Forward {
		localListener := listenOnNetworkingDevice(localConnection, f.port, spec.Logger)
		if localListener == nil {
			serverConnection.Close()
			return errors.New("could not open local port... closing down")
		}
		go acceptNewConnectionAndTunnel(context.Background(), localListener, serverConnection, f, spec.Logger, nil)
	}
	return nil
}

func ExecuteAndBlock(ctx context.Context, spec *Spec, ok chan<- struct{}) error {
	if spec.Logger == nil {
		spec.Logger = EmptyLogger()
	}
	config := getSSHConfig(spec)
	serverConnection, err := makeServerConnection(spec, config)
	if err != nil {
		return err
	}
	localConnection := localNetwork{}
	if spec.ForwardTimeout == 0 {
		spec.ForwardTimeout = time.Second * 5
	}
	localListeners := []net.Listener{}
	wg := sync.WaitGroup{}
	for _, f := range spec.Forward {
		localListener := listenOnNetworkingDevice(localConnection, f.port, spec.Logger)
		if localListener == nil {
			serverConnection.Close()
			return errors.New("could not open local port... closing down")
		}
		localListeners = append(localListeners, localListener)
		go acceptNewConnectionAndTunnel(ctx, localListener, serverConnection, f, spec.Logger, &wg)
	}
	remoteListeners := []net.Listener{}
	for _, f := range spec.Reverse {
		remoteListener := listenOnNetworkingDevice(serverConnection, f.port, spec.Logger)
		if remoteListener == nil {
			continue
		}
		remoteListeners = append(remoteListeners, remoteListener)
		go acceptNewConnectionAndTunnel(ctx, remoteListener, localConnection, f, spec.Logger, &wg)
	}
	serverConnectionDone := make(chan struct{})
	go func() {
		serverConnection.Wait()
		close(serverConnectionDone)
	}()
	close(ok)
	select {
	case <-ctx.Done():
		spec.Logger.Log("connection to %s terminating due to context cancellation", spec.Host)
		wg.Wait()
		spec.Logger.Log("all tunnels for %s are closed", spec.Host)
		for _, l := range localListeners {
			l.Close()
		}
		spec.Logger.Log("all local listeners for %s are closed", spec.Host)
		for _, l := range remoteListeners {
			l.Close()
		}
		spec.Logger.Log("all remote listeners for %s are closed", spec.Host)
		serverConnection.Close()
		serverConnection.Wait()
	case <-serverConnectionDone:
		spec.Logger.Log("%s terminated our connection", spec.Host)
		wg.Wait()
		spec.Logger.Log("all tunnels for %s are closed", spec.Host)
		for _, l := range localListeners {
			l.Close()
		}
		spec.Logger.Log("all listeners for %s are closed", spec.Host)
	}
	return nil
}

func monitor(serverConnection *ssh.Client, spec *Spec) {
	for {
		for _, f := range spec.Forward {
			if !isDestinationAvailable(serverConnection, f.destination, spec.ForwardTimeout) {
				spec.Logger.Log("%s is unreachable.", f.destination)
			}
		}
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
		Timeout: spec.ForwardTimeout,
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

type networkingDevice interface {
	Listen(network, address string) (net.Listener, error)
	Dial(n, addr string) (net.Conn, error)
}

type localNetwork struct{}

func (localNetwork) Listen(network, address string) (net.Listener, error) {
	return net.Listen(network, address)
}

func (localNetwork) Dial(n, addr string) (net.Conn, error) {
	return net.Dial(n, addr)
}

func listenOnNetworkingDevice(n networkingDevice, port int, logger Logger) net.Listener {
	conn, err := n.Listen("tcp", "localhost:"+strconv.Itoa(port))
	if err != nil {
		logger.Log("Unable to bind to remote port: %d\n", port)
		return nil
	}
	return conn
}

func acceptNewConnectionAndTunnel(ctx context.Context, listener net.Listener, destinationDevice networkingDevice, forwarder Forwarder, logger Logger, wg *sync.WaitGroup) {
	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
			default:
				logger.Log("Unable to accept new connection on port %d: %s\n", forwarder.port, err.Error())
			}
			return
		}
		logger.Log("Connection accepted on port: %d\n", forwarder.port)
		go tunnel(ctx, destinationDevice, conn, forwarder.destination, logger, wg)
	}
}

func tunnel(ctx context.Context, destinationDevice networkingDevice, localConnection net.Conn, destination string, logger Logger, wg *sync.WaitGroup) {
	remoteConnection, err := destinationDevice.Dial("tcp", destination)
	if err != nil {
		logger.Log("Unable to connect to remote destination %s: %s\n", destination, err.Error())
		localConnection.Close()
		return
	}
	logger.Log("\ttunneled connection from %s to %s established", localConnection.LocalAddr().String(), destination)

	if wg != nil {
		wg.Add(1)
	}
	localCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		<-localCtx.Done()
		remoteConnection.Close()
		localConnection.Close()
	}()

	nursery.RunConcurrently(
		func(context.Context, chan error) {
			n, err := io.Copy(localConnection, remoteConnection)
			logger.Log("\t\tfinished copying %d bytes from %s to %s", n, destination, localConnection.LocalAddr().String())
			if err != nil {
				logger.Log("error copying data from %s to %s: %v", destination, localConnection.LocalAddr().String(), err)
			}
			remoteConnection.Close()
			localConnection.Close()
		},
		func(context.Context, chan error) {
			n, err := io.Copy(remoteConnection, localConnection)
			logger.Log("\t\tfinished copying %d bytes from %s to %s", n, localConnection.LocalAddr().String(), destination)
			if err != nil {
				logger.Log("error copying data from %s to %s: %v", localConnection.LocalAddr().String(), destination, err)
			}
			remoteConnection.Close()
			localConnection.Close()
		},
	)
	logger.Log("\ttunneled connection from %s to %s terminated", localConnection.LocalAddr().String(), destination)
	if wg != nil {
		wg.Done()
	}
}

// PrivateKeyFile reads a private key and returns an AuthMethod using it
func PrivateKeyFile(file string, passPhrase string) (ssh.AuthMethod, error) {
	buffer, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, errors.New("Couldn't read private key:" + err.Error())
	}

	if passPhrase == "" {
		key, err := ssh.ParsePrivateKey(buffer)
		if err != nil {
			return nil, errors.New("Couldn't parse private key:" + err.Error())
		}
		return ssh.PublicKeys(key), nil
	} else {
		key, err := ssh.ParsePrivateKeyWithPassphrase(buffer, []byte(passPhrase))
		if err != nil {
			return nil, errors.New("Couldn't parse private key:" + err.Error())
		}
		return ssh.PublicKeys(key), nil
	}
}

// DEPRECATED - due to lack of timeout support on serverConnection.Dial
func isDestinationAvailable(serverConnection *ssh.Client, destination string, timeout time.Duration) bool {
	conn, err := serverConnection.Dial("tcp", destination)
	if err != nil {
		log.Println(err)
		return false
	}
	conn.Close()
	return true
}
