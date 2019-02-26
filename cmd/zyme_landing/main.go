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

	if keyLocation == "" || keyPwd == "" || e2openDevPwd == "" || zymePwd == "" {
		log.Fatal("Required keys & passwords not found. Did you do source env.sh?")
	}

	key, err := tunnel.PrivateKeyFile(keyLocation, keyPwd)
	if err != nil {
		log.Fatal(err)
	}
	pwd := ssh.Password(e2openDevPwd)
	pwd2 := ssh.Password(zymePwd)

	ch1 := make(chan struct{})
	connectToFrontChannel(key, pwd, ch1)
	ch2 := make(chan struct{})
	connectToDev1249(pwd, ch2)
	ch3 := make(chan struct{})
	connectToTimeOperations(pwd2, ch3)

	select {
	case <-ch1:
		return
	case <-ch2:
		return
	case <-ch3:
		return
	}
}

func connectToFrontChannel(key ssh.AuthMethod, pwd ssh.AuthMethod, die chan struct{}) {
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
		Die:    die,
	}
	if err := tunnel.Execute(spec); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Connection to bastion1.e2open.com successfully established...")
}

func connectToDev1249(pwd ssh.AuthMethod, die chan struct{}) {
	spec := &tunnel.Spec{
		Host: "localhost:1234",
		User: "abarua",
		Auth: []ssh.AuthMethod{
			pwd,
		},
		Forward: []tunnel.Forwarder{
			tunnel.Forward(1235, "172.16.1.191:22"),
		},
		Logger: tunnel.StdOutLogger(),
		Die:    die,
	}
	if err := tunnel.Execute(spec); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Connection to dev1249.dev.e2open.com successfully established...")
}

func connectToTimeOperations(pwd ssh.AuthMethod, die chan struct{}) {
	spec := &tunnel.Spec{
		Host: "localhost:1235",
		User: "abarua",
		Auth: []ssh.AuthMethod{
			pwd,
		},
		Forward: []tunnel.Forwarder{
			tunnel.Forward(2222, "10.101.101.89:22"),
			tunnel.Forward(2223, "10.107.107.188:22"),
			tunnel.Forward(8088, "10.107.107.102:8088"),
			tunnel.Forward(19888, "10.107.107.100:19888"),
		},
		Logger: tunnel.StdOutLogger(),
		Die:    die,
	}
	if err := tunnel.Execute(spec); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Connection to time.operations.local successfully established...")
	fmt.Printf("\tssh -i %s -p 2222 localhost  <-- to ssh to zyme front channel\n", keyLocation)
	fmt.Printf("\tssh -p 2223 localhost  <-- to ssh to zlakes34\n")
	fmt.Println("\thttp://localhost:19888/    <-- to access Hadoop Job History")
	fmt.Println("\thttp://localhost:8088/cluster/apps/RUNNING    <-- to access Hadoop Running Jobs")
}
