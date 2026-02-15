// Package main is the CLI entry point for the amazing-gitlab-exporter.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"

	"github.com/amazing-gitlab-exporter/amazing-gitlab-exporter/internal/config"
	"github.com/amazing-gitlab-exporter/amazing-gitlab-exporter/internal/exporter"
)

// Build-time variables set via -ldflags.
var (
	version = "dev"
	commit  = "none"
)

func main() {
	app := &cli.Command{
		Name:    "amazing-gitlab-exporter",
		Usage:   "Prometheus exporter for GitLab CI/CD and analytics metrics",
		Version: version,
		Commands: []*cli.Command{
			runCommand(),
			versionCommand(),
		},
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func runCommand() *cli.Command {
	return &cli.Command{
		Name:  "run",
		Usage: "Start the exporter",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"c"},
				Usage:   "Path to YAML configuration file",
				Value:   "",
				Sources: cli.EnvVars("AGE_CONFIG"),
			},
			&cli.StringFlag{
				Name:    "gitlab-url",
				Usage:   "GitLab instance URL",
				Sources: cli.EnvVars("AGE_GITLAB_URL"),
			},
			&cli.StringFlag{
				Name:    "gitlab-token",
				Usage:   "GitLab personal access token",
				Sources: cli.EnvVars("AGE_GITLAB_TOKEN"),
			},
			&cli.StringFlag{
				Name:    "log-level",
				Usage:   "Log level (trace, debug, info, warn, error, fatal, panic)",
				Value:   "info",
				Sources: cli.EnvVars("AGE_LOG_LEVEL"),
			},
			&cli.StringFlag{
				Name:    "server-listen-address",
				Usage:   "HTTP listen address (e.g. :8080)",
				Sources: cli.EnvVars("AGE_SERVER_LISTEN_ADDRESS"),
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			// --- Configure logging ---
			logger := logrus.New()
			level, err := logrus.ParseLevel(cmd.String("log-level"))
			if err != nil {
				level = logrus.InfoLevel
			}
			logger.SetLevel(level)
			logger.SetFormatter(&logrus.TextFormatter{
				FullTimestamp: true,
			})
			log := logger.WithField("app", "amazing-gitlab-exporter")

			// --- Load configuration ---
			var cfg *config.Config
			configPath := cmd.String("config")
			if configPath != "" {
				cfg, err = config.Load(configPath)
				if err != nil {
					return fmt.Errorf("loading config from %s: %w", configPath, err)
				}
			} else {
				cfg = &config.Config{}
			}

			// Apply defaults for any unset values.
			config.ApplyDefaults(cfg)

			// --- CLI overrides ---
			if v := cmd.String("gitlab-url"); v != "" {
				cfg.GitLab.URL = v
			}
			if v := cmd.String("gitlab-token"); v != "" {
				cfg.GitLab.Token = v
			}
			if v := cmd.String("server-listen-address"); v != "" {
				cfg.Server.ListenAddress = v
			}

			// --- Validate required fields ---
			if cfg.GitLab.URL == "" {
				return fmt.Errorf("gitlab URL is required (--gitlab-url or config file)")
			}
			if cfg.GitLab.Token == "" {
				return fmt.Errorf("gitlab token is required (--gitlab-token or config file)")
			}

			log.WithFields(logrus.Fields{
				"version": version,
				"commit":  commit,
			}).Info("starting amazing-gitlab-exporter")

			// --- Create exporter ---
			exp, err := exporter.NewExporter(cfg, log)
			if err != nil {
				return fmt.Errorf("initializing exporter: %w", err)
			}

			// --- OS signal handling for graceful shutdown ---
			ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
			defer stop()

			// --- Run ---
			return exp.Run(ctx)
		},
	}
}

func versionCommand() *cli.Command {
	return &cli.Command{
		Name:  "version",
		Usage: "Print version information",
		Action: func(_ context.Context, _ *cli.Command) error {
			fmt.Printf("amazing-gitlab-exporter %s (commit: %s)\n", version, commit)
			return nil
		},
	}
}
