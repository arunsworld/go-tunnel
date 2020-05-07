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

	if keyLocation == "" || keyPwd == "" || e2openDevPwd == "" {
		log.Fatal("Required keys & passwords not found. Did you do source env.sh?")
	}

	key, err := tunnel.PrivateKeyFile(keyLocation, keyPwd)
	if err != nil {
		log.Fatal(err)
	}
	pwd := ssh.Password(e2openDevPwd)

	if err := connectToFrontChannel(key, pwd); err != nil {
		log.Fatal(err)
	}
	if err := connectToApps(pwd); err != nil {
		log.Println(err)
	}

	select {}

}

func connectToFrontChannel(key ssh.AuthMethod, pwd ssh.AuthMethod) error {
	spec := &tunnel.Spec{
		Host: "bastion3.chg.e2open.com:2222",
		User: "abarua",
		Auth: []ssh.AuthMethod{
			key,
			pwd,
		},
		Forward: []tunnel.Forwarder{
			tunnel.Forward(2345, "10.140.241.166:22"),
			tunnel.Forward(2346, "10.140.241.160:22"),
			tunnel.Forward(2347, "10.140.241.167:22"),
			tunnel.Forward(2348, "10.140.241.168:22"),
		},
		Logger: tunnel.StdOutLogger(),
	}
	if err := tunnel.Execute(spec); err != nil {
		return fmt.Errorf("could not connect to front channel: %v", err)
	}
	log.Println("Connection to bastion3.chg.e2open.com successfully established...")
	log.Println("\tssh -p 2345 localhost   <-- to ssh to master (prod42166)")
	log.Println("\tssh -p 2346 localhost   <-- to ssh to jump server (prod42160)")
	log.Println("\tssh -p 2347 localhost   <-- to ssh to influxdb server (prod42167)")
	log.Println("\tssh -p 2348 localhost   <-- to ssh to awx server (prod42168)")
	return nil
}

func connectToApps(pwd ssh.AuthMethod) error {
	spec := &tunnel.Spec{
		Host: "localhost:2345",
		User: "abarua",
		Auth: []ssh.AuthMethod{
			pwd,
		},
		Forward: []tunnel.Forwarder{
			tunnel.Forward(3456, "localhost:3456"),
			tunnel.Forward(3556, "localhost:3556"),
			tunnel.Forward(8011, "10.140.241.168:8001"),
			tunnel.Forward(3000, "10.140.241.167:3000"),
			tunnel.Forward(9090, "10.140.241.167:9090"),
		},
		Logger: tunnel.StdOutLogger(),
	}
	if err := tunnel.Execute(spec); err != nil {
		return fmt.Errorf("could not connect to apps: %v", err)
	}
	log.Println("Connection to master successfully established...")
	log.Println("\thttp://localhost:3456/  <-- engine master")
	log.Println("\thttp://localhost:3556/  <-- engine master regression")
	log.Println("\thttp://localhost:8011/  <-- awx")
	log.Println("\thttp://localhost:3000/  <-- grafana")
	log.Println("\thttp://localhost:9090/  <-- prometheus")
	return nil
}
