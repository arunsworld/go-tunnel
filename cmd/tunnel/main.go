package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/urfave/cli/v2"
)

type config struct {
	configFile string
}

func main() {

	flags, conf := flagsAndConfig()

	app := &cli.App{
		Name:      "ssh tunnel",
		Usage:     "tunnel ports through an ssh connection",
		UsageText: "tunnel [options] <config file>",
		Flags:     flags,
		Action: func(ctx *cli.Context) error {
			if ctx.NArg() != 1 {
				return errors.New("confg file not provided")
			}
			conf.configFile = ctx.Args().First()
			return run(ctx.Context, conf)
		},
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := app.RunContext(ctx, os.Args); err != nil {
		log.Fatal(err)
	}
}

func flagsAndConfig() ([]cli.Flag, *config) {
	conf := config{}
	return []cli.Flag{}, &conf
}
