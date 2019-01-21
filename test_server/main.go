package main

import (
	"io"
	"io/ioutil"
	"log"

	"github.com/gliderlabs/ssh"
	cryptossh "golang.org/x/crypto/ssh"
)

func main() {

	pwdHandler := func(ctx ssh.Context, password string) bool {
		if ctx.User() != "testuser" {
			return false
		}
		if password != "the right password" {
			return false
		}
		return true
	}

	lc := ssh.LocalPortForwardingCallback(func(ctx ssh.Context, dhost string, dport uint32) bool {
		log.Println("Accepted forward", dhost, dport)
		return true
	})

	buffer, err := ioutil.ReadFile("id_rsa.pub")
	if err != nil {
		log.Fatal("Couldn't read public key:", err)
	}
	acceptableKey, _, _, _, err := cryptossh.ParseAuthorizedKey(buffer)
	if err != nil {
		log.Fatal("Couldn't parse public key:", err)
	}

	pkeyHandler := func(ctx ssh.Context, key ssh.PublicKey) bool {
		if ctx.User() != "testuser" {
			return false
		}
		return ssh.KeysEqual(key, acceptableKey)
	}

	server := ssh.Server{
		Addr: ":2222",
		Handler: ssh.Handler(func(s ssh.Session) {
			io.WriteString(s, "hello, world\n")
		}),
		PasswordHandler:             pwdHandler,
		LocalPortForwardingCallback: lc,
		PublicKeyHandler:            pkeyHandler,
	}

	log.Fatal(server.ListenAndServe())
}
