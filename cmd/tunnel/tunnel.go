package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"syscall"

	tunnel "github.com/arunsworld/go-tunnel"
	"github.com/arunsworld/nursery"
	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
	"gopkg.in/yaml.v2"
)

type tunnelConfig struct {
	Secrets    []secret
	SshConfigs []sshConfig `json:"sshconfigs"`
}

type secret struct {
	Name string
	Env  string
}

type secretsVault map[string]vaultSecret

func (s secretsVault) secretFor(k string) (vaultSecret, error) {
	v, ok := s[k]
	if !ok {
		return "", fmt.Errorf("secret %s not setup", k)
	}
	return v, nil
}

type vaultSecret string

func newSecretsVault(rawSecrets []secret) (secretsVault, error) {
	result := make(secretsVault)
	for _, s := range rawSecrets {
		sv, err := newSecretVault(s)
		if err != nil {
			return nil, err
		}
		result[s.Name] = sv
	}
	return result, nil
}

func newSecretVault(s secret) (vaultSecret, error) {
	if s.Name == "" {
		return "", fmt.Errorf("secret specified without a name")
	}
	if s.Env != "" {
		v := os.Getenv(s.Env)
		if v != "" {
			return vaultSecret(v), nil
		}
	}
	fmt.Printf("Enter value for secret %s: ", s.Name)
	bytePassword, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		return "", fmt.Errorf("error reading secret from prompt for %s: %v", s.Name, err)
	}
	if bytePassword == nil || string(bytePassword) == "" {
		return "", fmt.Errorf("error - no value provided for secret %s", s.Name)
	}
	fmt.Println("")
	return vaultSecret(string(bytePassword)), nil
}

type sshConfig struct {
	Destination string
	User        string
	Auth        []auth
	Tunnels     []portForward
	ThroughSSH  []sshConfig
}

func (sc *sshConfig) validateAndUpdateAuth(vault secretsVault) error {
	for i, a := range sc.Auth {
		if err := a.validateAndUpdate(vault); err != nil {
			return err
		}
		sc.Auth[i] = a
	}
	return nil
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

func (a *auth) validateAndUpdate(vault secretsVault) error {
	if err := a.KeyAuth.validateAndUpdate(vault); err != nil {
		return err
	}
	if err := a.PwdAuth.validateAndUpdate(vault); err != nil {
		return err
	}
	return nil
}

type keyAuth struct {
	FileLocation   string
	PasswordSecret string
	// internal
	password vaultSecret
}

func (a *keyAuth) validateAndUpdate(vault secretsVault) error {
	if a.PasswordSecret != "" {
		v, err := vault.secretFor(a.PasswordSecret)
		if err != nil {
			return err
		}
		a.password = v
	}
	return nil
}

type pwdAuth struct {
	PasswordSecret string
	// internal
	password vaultSecret
}

func (a *pwdAuth) validateAndUpdate(vault secretsVault) error {
	if a.PasswordSecret != "" {
		v, err := vault.secretFor(a.PasswordSecret)
		if err != nil {
			return err
		}
		a.password = v
	}
	return nil
}

func (sc *sshConfig) validateAndUpdate(vault secretsVault) error {
	if sc.Destination == "" {
		return fmt.Errorf("config has empty destination")
	}
	if err := sc.validateAndUpdateAuth(vault); err != nil {
		return err
	}
	for i, pf := range sc.ThroughSSH {
		if pf.Destination == "" {
			return fmt.Errorf("ThroughSSH config has empty destination")
		}
		if err := pf.validateAndUpdateAuth(vault); err != nil {
			return err
		}
		sc.ThroughSSH[i] = pf
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
	tunnelConf := tunnelConfig{}
	if err := yaml.Unmarshal(contents, &tunnelConf); err != nil {
		return fmt.Errorf("unable to parse config file %s: %v", conf.configFile, err)
	}
	vault, err := newSecretsVault(tunnelConf.Secrets)
	if err != nil {
		return err
	}
	jobs := []nursery.ConcurrentJob{}
	for i, c := range tunnelConf.SshConfigs {
		if err := c.validateAndUpdate(vault); err != nil {
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
		if auth.KeyAuth.PasswordSecret != "" {
			pwd = string(auth.KeyAuth.password)
		}
		key, err := tunnel.PrivateKeyFile(auth.KeyAuth.FileLocation, pwd)
		if err != nil {
			return nil, fmt.Errorf("unable to create private key auth: %v", err)
		}
		return key, nil
	case auth.PwdAuth.PasswordSecret != "":
		return ssh.Password(string(auth.PwdAuth.password)), nil
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
