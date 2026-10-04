#!/usr/bin/env node
import { Command } from 'commander';
import { loadConfig } from '../core/config.js';
import { createLogger } from '../core/logger.js';
import { createDemoService } from '../services/demoService.js';
import { runCommand } from './commands/run.js';
import { serveCommand } from './commands/serve.js';

const program = new Command();

program
  .name('app')
  .description('JS Starter CLI')
  .version('1.0.0')
  .option('-e, --env <env>', 'environment (development|testing|production)')
  .option('-l, --log-level <level>', 'log level (debug|info|warn|error)')
  .option('-h, --host <host>', 'server host')
  .option('-p, --port <port>', 'server port')
  .option('--serve', 'start server mode');

program
  .command('run')
  .description('Run core application')
  .action(async () => {
    try {
      const opts = program.opts();
      const overrides = {};
      if (opts.env) overrides.nodeEnv = opts.env;
      if (opts.logLevel) overrides.logLevel = opts.logLevel;
      if (opts.host) overrides.host = opts.host;
      if (opts.port) overrides.port = parseInt(opts.port, 10);

      const config = loadConfig(overrides);
      const logger = createLogger(config);
      const demoService = createDemoService(logger);
      const services = { demoService };

      await runCommand(config, logger, services);
      logger.flush?.();
    } catch (error) {
      console.error(error.message);
      process.exit(1);
    }
  });

program
  .command('serve')
  .description('Start REST API server')
  .action(async () => {
    try {
      const opts = program.opts();
      const overrides = {};
      if (opts.env) overrides.nodeEnv = opts.env;
      if (opts.logLevel) overrides.logLevel = opts.logLevel;
      if (opts.host) overrides.host = opts.host;
      if (opts.port) overrides.port = parseInt(opts.port, 10);

      const config = loadConfig(overrides);
      const logger = createLogger(config);
      const demoService = createDemoService(logger);
      const services = { demoService };

      await serveCommand(config, logger, services);
    } catch (error) {
      console.error(error.message);
      process.exit(1);
    }
  });

async function main() {
  const opts = program.opts();

  if (opts.serve) {
    try {
      const overrides = {};
      if (opts.env) overrides.nodeEnv = opts.env;
      if (opts.logLevel) overrides.logLevel = opts.logLevel;
      if (opts.host) overrides.host = opts.host;
      if (opts.port) overrides.port = parseInt(opts.port, 10);

      const config = loadConfig(overrides);
      const logger = createLogger(config);
      const demoService = createDemoService(logger);
      const services = { demoService };

      await serveCommand(config, logger, services);
      return;
    } catch (error) {
      console.error(error.message);
      process.exit(1);
    }
  }

  await program.parseAsync(process.argv);
}

main().catch((error) => {
  console.error(error.message);
  process.exit(1);
});
