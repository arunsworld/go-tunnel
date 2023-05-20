package main

import (
	"context"
	"fmt"
	"log"
	"os"

	tunnel "github.com/arunsworld/go-tunnel"
	"github.com/arunsworld/nursery"
	"golang.org/x/crypto/ssh"
	"gopkg.in/yaml.v2"
)

type sshConfig struct {
	Destination string
	User        string
	Auth        []auth
	Tunnels     []portForward
	ThroughSSH  []sshConfig
}

type portForward struct {
	Name   string
	Port   int
	Target string
	Ignore bool
}

type auth struct {
	KeyAuth keyAuth
	PwdAuth pwdAuth
}

type keyAuth struct {
	FileLocation string
	PasswordEnv  string
}

type pwdAuth struct {
	PasswordEnv string
}

func (sc *sshConfig) validate() error {
	if sc.Destination == "" {
		return fmt.Errorf("config has empty destination")
	}
	for _, pf := range sc.ThroughSSH {
		if pf.Destination == "" {
			return fmt.Errorf("ThroughSSH config has empty destination")
		}
	}
	return nil
}

func (sc *sshConfig) logSuccessful() {
	log.Printf("Connection to %s successfully established...", sc.Destination)
	for _, f := range sc.Tunnels {
		if f.Ignore {
			continue
		}
		log.Printf("\testablished tunnel %s: forwarded %d to %s", f.Name, f.Port, f.Target)
	}
}

func run(ctx context.Context, conf *config) error {
	if conf.configFile == "" {
		return fmt.Errorf("cannot proceed without config file")
	}
	contents, err := os.ReadFile(conf.configFile)
	if err != nil {
		return fmt.Errorf("unable to open config file %s: %v", conf.configFile, err)
	}
	sshConf := []sshConfig{}
	if err := yaml.Unmarshal(contents, &sshConf); err != nil {
		return fmt.Errorf("unable to parse config file %s: %v", conf.configFile, err)
	}
	jobs := []nursery.ConcurrentJob{}
	for i, c := range sshConf {
		if err := c.validate(); err != nil {
			return fmt.Errorf("invalid config #%d: %v", i, err)
		}
		jobs = append(jobs, jobForConfig(ctx, c))
	}
	if len(jobs) == 0 {
		return fmt.Errorf("no successfull connections, terminating")
	}
	return nursery.RunConcurrently(jobs...)
}

func jobForConfig(ctx context.Context, conf sshConfig) nursery.ConcurrentJob {
	return func(_ context.Context, _ chan error) {
		if err := handleConnectionTo(ctx, conf); err != nil {
			log.Printf("error connecting to %s: %v", conf.Destination, err)
		}
	}
}

func sshAuthFromAuth(auth auth) (ssh.AuthMethod, error) {
	switch {
	case auth.KeyAuth.FileLocation != "":
		pwd := ""
		if auth.KeyAuth.PasswordEnv != "" {
			pwd = os.Getenv(auth.KeyAuth.PasswordEnv)
			if pwd == "" {
				return nil, fmt.Errorf("env variable %s not set", auth.KeyAuth.PasswordEnv)
			}
		}
		key, err := tunnel.PrivateKeyFile(auth.KeyAuth.FileLocation, pwd)
		if err != nil {
			return nil, fmt.Errorf("unable to create private key auth: %v", err)
		}
		return key, nil
	case auth.PwdAuth.PasswordEnv != "":
		pwd := os.Getenv(auth.PwdAuth.PasswordEnv)
		if pwd == "" {
			return nil, fmt.Errorf("env variable %s not set", auth.PwdAuth.PasswordEnv)
		}
		return ssh.Password(pwd), nil
	default:
		return nil, fmt.Errorf("invalid auth details")
	}
}

func handleConnectionTo(ctx context.Context, conf sshConfig) error {
	spec := &tunnel.Spec{
		Host:   conf.Destination,
		User:   conf.User,
		Logger: tunnel.StdOutLogger(),
	}
	for _, auth := range conf.Auth {
		sshAuth, err := sshAuthFromAuth(auth)
		if err != nil {
			return err
		}
		spec.Auth = append(spec.Auth, sshAuth)
	}
	for _, f := range conf.Tunnels {
		if f.Ignore {
			continue
		}
		spec.Forward = append(spec.Forward, tunnel.Forward(f.Port, f.Target))
	}
	ok := make(chan struct{})
	finished := make(chan struct{})
	return nursery.RunConcurrently(
		func(_ context.Context, errCh chan error) {
			if err := tunnel.ExecuteAndBlock(ctx, spec, ok); err != nil {
				errCh <- err
			}
			close(finished)
		},
		func(context.Context, chan error) {
			select {
			case <-ok:
				conf.logSuccessful()
			case <-finished:
				return
			}
			jobs := []nursery.ConcurrentJob{}
			for _, c := range conf.ThroughSSH {
				jobs = append(jobs, jobForConfig(ctx, c))
			}
			nursery.RunConcurrently(jobs...)
		},
	)
}
