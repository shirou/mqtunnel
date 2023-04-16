package main

import (
	"context"
	"log"
	"os"

	"github.com/shirou/mqtunnel"
	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
)

func main() {
	app := &cli.App{
		Name:  "mqtunnel",
		Usage: "tunnel via MQTT",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "conf",
				Aliases:  []string{"c"},
				Value:    "config.json",
				Usage:    "config file",
				Required: true,
			},
			&cli.BoolFlag{
				Name:    "verbose",
				Aliases: []string{"v"},
				Value:   false,
				Usage:   "verbose",
			},
			&cli.IntFlag{
				Name:    "local",
				Aliases: []string{"l"},
				Value:   0,
				Usage:   "local port",
			},
			&cli.IntFlag{
				Name:    "remote",
				Aliases: []string{"r"},
				Value:   0,
				Usage:   "remote port",
			},
		},
		Action: func(cCtx *cli.Context) error {
			logger := setupLog(cCtx.Bool("verbose"))
			defer logger.Sync()
			undo := zap.ReplaceGlobals(logger)
			defer undo()

			conf, err := mqtunnel.ReadConfig(cCtx.String("conf"))
			if err != nil {
				return err
			}
			tun, err := mqtunnel.NewMQTunnel(conf)
			if err != nil {
				return err
			}

			local := cCtx.Int("local")
			remote := cCtx.Int("remote")
			tunel := mqtunnel.NewTunnel(conf, local, remote)

			ctx := context.Background()

			return tun.Start(ctx, tunel)
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
