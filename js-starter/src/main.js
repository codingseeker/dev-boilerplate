#!/usr/bin/env node
import { loadConfig } from './core/config.js';
import { createLogger } from './core/logger.js';
import { createDemoService } from './services/demoService.js';
import { runCommand } from './cli/commands/run.js';
import { serveCommand } from './cli/commands/serve.js';

const args = process.argv.slice(2);
const serveMode = args.includes('--serve');

async function main() {
  try {
    const config = loadConfig();
    const logger = createLogger(config);
    const demoService = createDemoService(logger);
    const services = { demoService };

    if (serveMode) {
      await serveCommand(config, logger, services);
    } else {
      await runCommand(config, logger, services);
    }
  } catch (error) {
    console.error(error.message);
    process.exit(1);
  }
}

main();
