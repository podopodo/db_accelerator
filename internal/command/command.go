// Package command implements the accelerator command-line interface.
package command

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/podopodo/db_accelerator/internal/app"
	"github.com/podopodo/db_accelerator/internal/buildinfo"
	"github.com/podopodo/db_accelerator/internal/config"
	"github.com/podopodo/db_accelerator/internal/upstream"
)

const usage = `Database Accelerator

Usage:
  accelerator version
  accelerator config init [options]
  accelerator config validate [options]
  accelerator doctor [options]
  accelerator serve [options]
`

// Run executes one command and returns a process exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stdout, usage)
		return 0
	}

	switch args[0] {
	case "version", "--version", "-version":
		fmt.Fprintln(stdout, buildinfo.Current().String())
		return 0
	case "help", "--help", "-h":
		fmt.Fprint(stdout, usage)
		return 0
	case "config":
		return runConfig(args[1:], stdout, stderr)
	case "doctor":
		return runDoctor(args[1:], stdout, stderr)
	case "serve":
		return runServe(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n%s", args[0], usage)
		return 2
	}
}

func runDoctor(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("doctor", flag.ContinueOnError)
	flags.SetOutput(stderr)
	path := flags.String("config", "accelerator.yaml", "base configuration path")
	managed := flags.String("managed-config", "accelerator.managed.yaml", "managed overlay path")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	cfg, err := config.Load(config.LoadOptions{Path: *path, ManagedPath: *managed})
	if err != nil {
		fmt.Fprintf(stderr, "configuration invalid: %v\n", err)
		return 1
	}
	secrets, err := config.ResolveSecrets(cfg, os.LookupEnv)
	if err != nil {
		fmt.Fprintf(stderr, "configuration secret invalid: %v\n", err)
		return 1
	}
	connector, err := upstream.New(cfg, secrets)
	if err != nil {
		fmt.Fprintf(stderr, "doctor: %v\n", err)
		return 1
	}
	report, err := connector.Probe(context.Background())
	if err != nil {
		fmt.Fprintf(stderr, "doctor: %v\n", err)
		return 1
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		fmt.Fprintf(stderr, "encode doctor report: %v\n", err)
		return 1
	}
	return 0
}

func runConfig(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "config requires init or validate")
		return 2
	}
	switch args[0] {
	case "init":
		flags := flag.NewFlagSet("config init", flag.ContinueOnError)
		flags.SetOutput(stderr)
		output := flags.String("output", "accelerator.yaml", "output configuration path")
		force := flags.Bool("force", false, "replace an existing file")
		if err := flags.Parse(args[1:]); err != nil {
			return 2
		}
		if err := config.WriteDefault(*output, *force); err != nil {
			fmt.Fprintf(stderr, "write configuration: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "wrote %s\n", *output)
		return 0
	case "validate":
		flags := flag.NewFlagSet("config validate", flag.ContinueOnError)
		flags.SetOutput(stderr)
		path := flags.String("config", "accelerator.yaml", "base configuration path")
		managed := flags.String("managed-config", "accelerator.managed.yaml", "managed overlay path")
		if err := flags.Parse(args[1:]); err != nil {
			return 2
		}
		cfg, err := config.Load(config.LoadOptions{Path: *path, ManagedPath: *managed})
		if err != nil {
			fmt.Fprintf(stderr, "configuration invalid: %v\n", err)
			return 1
		}
		if _, err := config.ResolveSecrets(cfg, os.LookupEnv); err != nil {
			fmt.Fprintf(stderr, "configuration secret invalid: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "configuration valid (version %d)\n", cfg.Version)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown config command %q\n", args[0])
		return 2
	}
}

func runServe(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	flags.SetOutput(stderr)
	path := flags.String("config", "accelerator.yaml", "base configuration path")
	managed := flags.String("managed-config", "accelerator.managed.yaml", "managed overlay path")
	mysqlListen := flags.String("mysql-listen", "", "override MySQL listener")
	adminListen := flags.String("admin-listen", "", "override admin listener")
	dataDir := flags.String("data-dir", "", "override data directory")
	logLevel := flags.String("log-level", "", "override log level")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	overrides := config.Overrides{}
	flags.Visit(func(value *flag.Flag) {
		switch value.Name {
		case "mysql-listen":
			overrides.MySQLListen = mysqlListen
		case "admin-listen":
			overrides.AdminListen = adminListen
		case "data-dir":
			overrides.DataDir = dataDir
		case "log-level":
			overrides.LogLevel = logLevel
		}
	})

	cfg, err := config.Load(config.LoadOptions{
		Path:        *path,
		ManagedPath: *managed,
		Overrides:   overrides,
	})
	if err != nil {
		fmt.Fprintf(stderr, "configuration invalid: %v\n", err)
		return 1
	}
	secrets, err := config.ResolveSecrets(cfg, os.LookupEnv)
	if err != nil {
		fmt.Fprintf(stderr, "configuration secret invalid: %v\n", err)
		return 1
	}

	logger := slog.New(slog.NewJSONHandler(stderr, &slog.HandlerOptions{Level: parseLogLevel(cfg.Logging.Level)}))
	application := app.New(cfg, secrets, logger)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Fprintf(stdout, "Database Accelerator %s starting\n", buildinfo.Version)
	if err := application.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("accelerator stopped with error", "error", err)
		return 1
	}
	return 0
}

func parseLogLevel(value string) slog.Level {
	switch strings.ToLower(value) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
