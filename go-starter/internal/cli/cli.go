// Package cli translates command line arguments into application calls. It
// contains no business logic and keeps every side effect injectable.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/codingseeker/go-starter/internal/app"
	"github.com/codingseeker/go-starter/internal/config"
	"github.com/codingseeker/go-starter/internal/logger"
	"github.com/codingseeker/go-starter/internal/platform/dotenv"
	"github.com/codingseeker/go-starter/internal/server"
	"github.com/codingseeker/go-starter/internal/service"
)

const programName = "go-starter"

// Exit codes. They are part of the contract with scripts and CI jobs.
const (
	ExitOK    = 0
	ExitError = 1
	ExitUsage = 2
)

// UsageError marks an error caused by malformed input. Callers translate it
// into ExitUsage.
type UsageError struct {
	Err error
}

func (e *UsageError) Error() string { return e.Err.Error() }
func (e *UsageError) Unwrap() error { return e.Err }

func usagef(format string, args ...any) error {
	return &UsageError{Err: fmt.Errorf(format, args...)}
}

// CLI parses arguments and translates them into application calls. It contains
// no business logic and never touches global state directly: environment
// access, signal handling and output writers are injected, so the whole command
// surface is testable in process.
type CLI struct {
	// Out and Err receive user facing output.
	Out io.Writer
	Err io.Writer
	// Args includes the program name as the first element.
	Args []string
	// Getenv resolves environment variables; defaults to os.Getenv.
	Getenv func(string) string
	// SignalContext is handed to the application layer; defaults to real
	// signal handling.
	SignalContext app.SignalContext
	// LoadEnvFile applies a dotenv file; defaults to dotenv.Load. Tests inject
	// a loader so no test needs to mutate the process environment.
	LoadEnvFile func(path string) (bool, error)
	// Version is reported by the version command.
	Version string
}

// New returns a CLI wired to the process environment and standard streams.
func New(args []string, version string) *CLI {
	return &CLI{
		Out:           os.Stdout,
		Err:           os.Stderr,
		Args:          args,
		Getenv:        os.Getenv,
		SignalContext: app.NotifyContext,
		LoadEnvFile:   dotenv.Load,
		Version:       version,
	}
}

func (c *CLI) getenv(key string) string {
	if c.Getenv == nil {
		return os.Getenv(key)
	}
	return c.Getenv(key)
}

// Run executes the requested command.
func (c *CLI) Run(ctx context.Context) error {
	command, args := "", []string(nil)
	if len(c.Args) > 1 {
		command = strings.TrimSpace(c.Args[1])
		args = c.Args[2:]
	}

	switch command {
	case "":
		c.printUsage(c.Err)
		return usagef("no command given")
	case "help", "--help", "-h":
		c.printUsage(c.Out)
		return nil
	case "version", "--version", "-v":
		_, _ = fmt.Fprintf(c.Out, "%s %s\n", programName, c.version())
		return nil
	case "run":
		return c.run(ctx, args)
	case "serve":
		return c.serve(ctx, args)
	default:
		c.printUsage(c.Err)
		return usagef("unknown command %q", command)
	}
}

func (c *CLI) version() string {
	if strings.TrimSpace(c.Version) == "" {
		return "dev"
	}
	return c.Version
}

// options collects configuration flags for both commands. Only flags that were
// actually set override a value, which keeps the precedence
// "flag > environment > .env file > default" observable.
type options struct {
	appName         string
	environment     string
	logLevel        string
	httpHost        string
	httpPort        int
	readTimeout     durationFlag
	writeTimeout    durationFlag
	idleTimeout     durationFlag
	shutdownTimeout durationFlag
	envFile         string
	envFileSet      bool
}

func (o *options) register(fs *flag.FlagSet) {
	fs.StringVar(&o.appName, "app-name", "", "application name ("+config.EnvAppName+")")
	fs.StringVar(&o.environment, "env", "", "environment: development, testing or production ("+config.EnvEnvironment+")")
	fs.StringVar(&o.logLevel, "log-level", "", "log level: debug, info, warn or error ("+config.EnvLogLevel+")")
	fs.StringVar(&o.httpHost, "http-host", "", "http listen host ("+config.EnvHTTPHost+")")
	fs.IntVar(&o.httpPort, "http-port", 0, "http listen port ("+config.EnvHTTPPort+")")
	fs.Var(&o.readTimeout, "http-read-timeout", "http read timeout such as 30s ("+config.EnvReadTimeout+")")
	fs.Var(&o.writeTimeout, "http-write-timeout", "http write timeout ("+config.EnvWriteTimeout+")")
	fs.Var(&o.idleTimeout, "http-idle-timeout", "http idle timeout ("+config.EnvIdleTimeout+")")
	fs.Var(&o.shutdownTimeout, "http-shutdown-timeout", "graceful shutdown timeout ("+config.EnvShutdownTimeout+")")
	fs.StringVar(&o.envFile, "env-file", "", "path to a .env file, refused while "+config.EnvEnvironment+" is production")
}

// parseFlags parses the flags of a command. helpRequested is reported
// separately so that "serve --help" stays a successful command.
func (c *CLI) parseFlags(name string, args []string) (opts *options, helpRequested bool, err error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	// Errors are reported once, wrapped with the command name, so the output of
	// the flag package itself is discarded.
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}

	opts = &options{}
	opts.register(fs)

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil, true, nil
		}
		return nil, false, &UsageError{Err: fmt.Errorf("parse flags for %q: %w", name, err)}
	}
	if fs.NArg() > 0 {
		return nil, false, usagef("%s does not accept positional arguments, got %q", name, strings.Join(fs.Args(), " "))
	}
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "env-file" {
			opts.envFileSet = true
		}
	})
	return opts, false, nil
}

// resolveConfig applies the documented precedence and validates the result.
func (c *CLI) resolveConfig(opts *options) (config.Config, error) {
	if err := c.loadEnvFile(opts); err != nil {
		return config.Config{}, err
	}

	values := make(map[string]string, len(config.Keys()))
	for _, key := range config.Keys() {
		if value := strings.TrimSpace(c.getenv(key)); value != "" {
			values[key] = value
		}
	}

	overrides := map[string]string{
		config.EnvAppName:         opts.appName,
		config.EnvEnvironment:     opts.environment,
		config.EnvLogLevel:        opts.logLevel,
		config.EnvHTTPHost:        opts.httpHost,
		config.EnvReadTimeout:     opts.readTimeout.String(),
		config.EnvWriteTimeout:    opts.writeTimeout.String(),
		config.EnvIdleTimeout:     opts.idleTimeout.String(),
		config.EnvShutdownTimeout: opts.shutdownTimeout.String(),
	}
	if opts.httpPort != 0 {
		overrides[config.EnvHTTPPort] = fmt.Sprintf("%d", opts.httpPort)
	}
	for key, value := range overrides {
		if strings.TrimSpace(value) == "" {
			continue
		}
		values[key] = value
	}

	cfg, err := config.LoadFromEnv(func(key string) string { return values[key] })
	if err != nil {
		return config.Config{}, &UsageError{Err: err}
	}
	return cfg, nil
}

// loadEnvFile applies the .env file unless the resolved environment is
// production. Variables already present in the environment always win.
func (c *CLI) loadEnvFile(opts *options) error {
	if opts.envFileSet && strings.TrimSpace(opts.envFile) == "" {
		return usagef("--env-file requires a path")
	}
	path := opts.envFile
	if path == "" {
		path = ".env"
	}

	environment := strings.ToLower(strings.TrimSpace(opts.environment))
	if environment == "" {
		environment = strings.ToLower(strings.TrimSpace(c.getenv(config.EnvEnvironment)))
	}
	if environment == config.EnvironmentProduction {
		if opts.envFileSet {
			return usagef("refusing to load %s while %s is production; pass configuration through the environment",
				path, config.EnvEnvironment)
		}
		return nil
	}

	loader := c.LoadEnvFile
	if loader == nil {
		loader = dotenv.Load
	}
	loaded, err := loader(filepath.Clean(path))
	if err != nil {
		return &UsageError{Err: err}
	}
	if loaded {
		_, _ = fmt.Fprintf(c.Err, "loaded configuration from %s\n", path)
	}
	return nil
}

// prepared holds the shared dependencies of both commands, so the CLI and the
// API run on exactly the same configuration, logger and service.
type prepared struct {
	config  config.Config
	logger  *slog.Logger
	service *service.ApplicationService
}

func (c *CLI) prepare(name string, args []string) (*prepared, bool, error) {
	opts, helpRequested, err := c.parseFlags(name, args)
	if err != nil || helpRequested {
		return nil, helpRequested, err
	}

	cfg, err := c.resolveConfig(opts)
	if err != nil {
		return nil, false, err
	}

	log, err := logger.New(cfg, c.Err)
	if err != nil {
		return nil, false, fmt.Errorf("initialize logger: %w", err)
	}

	return &prepared{config: cfg, logger: log, service: service.New(cfg)}, false, nil
}

func (c *CLI) run(ctx context.Context, args []string) error {
	deps, helpRequested, err := c.prepare("run", args)
	if helpRequested {
		c.printCommandUsage(c.Out, "run")
		return nil
	}
	if err != nil {
		return err
	}

	application := app.New(deps.config, deps.logger, deps.service, app.WithSignalContext(c.SignalContext))
	return application.Run(logger.WithContext(ctx, deps.logger))
}

func (c *CLI) serve(ctx context.Context, args []string) error {
	deps, helpRequested, err := c.prepare("serve", args)
	if helpRequested {
		c.printCommandUsage(c.Out, "serve")
		return nil
	}
	if err != nil {
		return err
	}

	httpServer := server.New(deps.config, deps.logger, deps.service)
	application := app.New(deps.config, deps.logger, deps.service,
		app.WithSignalContext(c.SignalContext),
		// Readiness reports draining before the listener disappears, and the
		// listener itself is drained last with in-flight requests.
		app.WithCloser(func(context.Context) error { return deps.service.Close() }),
		app.WithCloser(httpServer.Shutdown),
	)

	return application.Serve(logger.WithContext(ctx, deps.logger), httpServer)
}

func (c *CLI) printUsage(w io.Writer) {
	_, _ = fmt.Fprintf(w, "%s - production grade Go boilerplate\n\n", programName)
	_, _ = fmt.Fprintf(w, "Usage:\n  %s <command> [flags]\n\n", programName)
	_, _ = fmt.Fprintf(w, "Commands:\n")
	_, _ = fmt.Fprintf(w, "  run       Run the core application\n")
	_, _ = fmt.Fprintf(w, "  serve     Start the HTTP REST API server\n")
	_, _ = fmt.Fprintf(w, "  version   Print the version and exit\n")
	_, _ = fmt.Fprintf(w, "  help      Print this help and exit\n\n")
	_, _ = fmt.Fprintf(w, "Global flags:\n")
	_, _ = fmt.Fprintf(w, "  -h, --help      Print this help and exit\n")
	_, _ = fmt.Fprintf(w, "  -v, --version   Print the version and exit\n\n")
	_, _ = fmt.Fprintf(w, "Configuration precedence: flags > environment variables > .env file (outside production) > defaults.\n")
	_, _ = fmt.Fprintf(w, "Run \"%s <command> -h\" to list the flags of a command.\n", programName)
}

func (c *CLI) printCommandUsage(w io.Writer, name string) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(w)
	opts := &options{}
	opts.register(fs)

	_, _ = fmt.Fprintf(w, "Usage:\n  %s %s [flags]\n\nFlags:\n", programName, name)
	fs.VisitAll(func(f *flag.Flag) {
		_, _ = fmt.Fprintf(w, "  --%-22s %s\n", f.Name, f.Usage)
	})
}

// ExitCode maps an error returned by Run onto a process exit code.
func ExitCode(err error) int {
	if err == nil {
		return ExitOK
	}
	var usageErr *UsageError
	if errors.As(err, &usageErr) {
		return ExitUsage
	}
	return ExitError
}
