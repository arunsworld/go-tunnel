package main

import (
	"fmt"
	"log"
	"os"

	tunnel "github.com/arunsworld/go-tunnel"
	"golang.org/x/crypto/ssh"
)

var (
	keyLocation string
)

func main() {
	keyLocation = os.Getenv("PRIVATE_KEY_LOC")
	keyPwd := os.Getenv("PRIVATE_KEY_PWD")
	e2openDevPwd := os.Getenv("E2OPEN_DEV_PWD")
	zymePwd := os.Getenv("ZYME_PWD")
	newOptimaPwd := os.Getenv("NEWOPTIMA_PWD")

	if keyLocation == "" || keyPwd == "" || e2openDevPwd == "" || zymePwd == "" || newOptimaPwd == "" {
		log.Fatal("Required keys & passwords not found. Did you do source env.sh?")
	}

	key, err := tunnel.PrivateKeyFile(keyLocation, keyPwd)
	if err != nil {
		log.Fatal(err)
	}

	if err := connectToFrontChannel(key, ssh.Password(e2openDevPwd)); err != nil {
		log.Fatal(err)
	}
	if err := connectToDev1249(ssh.Password(e2openDevPwd)); err != nil {
		log.Fatal(err)
	}
	if err := connectToAppsOnNewOptima(ssh.Password(newOptimaPwd)); err != nil {
		log.Println(err)
	}

	// wait forever
	select {}
}

func connectToFrontChannel(key ssh.AuthMethod, pwd ssh.AuthMethod) error {
	spec := &tunnel.Spec{
		Host: "bastion1.e2open.net:2222",
		User: "abarua",
		Auth: []ssh.AuthMethod{
			key,
			pwd,
		},
		Forward: []tunnel.Forwarder{
			tunnel.Forward(1234, "dev1249.dev.e2open.com:22"),
		},
		Logger: tunnel.StdOutLogger(),
	}
	if err := tunnel.Execute(spec); err != nil {
		return fmt.Errorf("could not connect to front channel: %v", err)
	}
	fmt.Println("Connection to bastion1.e2open.com successfully established...")
	fmt.Println("Forwarded 1234 to dev1249...")
	fmt.Printf("\tssh -p 1234 localhost  <-- to ssh to dev1249\n")
	return nil
}

func connectToDev1249(pwd ssh.AuthMethod) error {
	spec := &tunnel.Spec{
		Host: "localhost:1234",
		User: "abarua",
		Auth: []ssh.AuthMethod{
			pwd,
		},
		Forward: []tunnel.Forwarder{
			tunnel.Forward(1235, "172.16.1.191:22"), // ioccdm001.zymesolutions.local - current optima
			tunnel.Forward(1236, "10.34.224.34:22"), // new optima
			tunnel.Forward(1237, "172.16.2.120:22"), // tp devint box
		},
		Logger: tunnel.StdOutLogger(),
	}
	if err := tunnel.Execute(spec); err != nil {
		return fmt.Errorf("could not connect to dev1249: %v", err)
	}
	fmt.Println("Connection to dev1249.dev.e2open.com successfully established...")
	fmt.Println("Forwarded 1235 to 172.16.1.191, 1236 to 10.34.224.34, 1237 to 172.16.2.120...")
	fmt.Printf("\tssh -p 1235 localhost  <-- to ssh to ioccdm001.zymesolutions.local\n")
	fmt.Printf("\tssh -p 1236 localhost  <-- to ssh to new optima, awx\n")
	fmt.Printf("\tssh -p 1237 localhost  <-- to ssh to tp devint\n")
	return nil
}

func connectToAppsOnNewOptima(pwd ssh.AuthMethod) error {
	spec := &tunnel.Spec{
		Host: "localhost:1236",
		User: "abarua",
		Auth: []ssh.AuthMethod{
			pwd,
		},
		Forward: []tunnel.Forwarder{
			tunnel.Forward(9090, "localhost:9090"), // sca
		},
		Logger: tunnel.StdOutLogger(),
	}
	if err := tunnel.Execute(spec); err != nil {
		return fmt.Errorf("could not connect to apps on new optima: %v", err)
	}
	fmt.Println("Connection to new optima (10.34.224.34) successfully established...")
	fmt.Println("Forwarded 9090 to localhost:9090")
	fmt.Printf("\thttp://localhost:9090/  <-- to access sca\n")
	return nil
}
