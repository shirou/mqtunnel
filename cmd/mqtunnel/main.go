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
			&cli.StringFlag{
				Name:    "control",
				Aliases: []string{"C"},
				Usage:   "overwrite control topic",
			},
			&cli.IntFlag{
				Name:    "local",
				Aliases: []string{"l"},
				Usage:   "local port",
			},
			&cli.IntFlag{
				Name:    "remote",
				Aliases: []string{"r"},
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
			// overwrite control topic if the flag is specified.
			control := cCtx.String("control")
			if control != "" {
				conf.Control = control
			}

			mqt, err := mqtunnel.NewMQTunnel(conf)
			if err != nil {
				return err
			}

			local := cCtx.Int("local")
			remote := cCtx.Int("remote")

			ctx := context.Background()

			return mqt.Start(ctx, local, remote)
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
