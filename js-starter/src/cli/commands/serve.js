export async function serveCommand(config, logger, services) {
  const { createApp } = await import('../../api/app.js');
  const fastify = await createApp(config, logger, services);

  let shutdownRequested = false;

  const shutdown = async (signal) => {
    if (shutdownRequested) {
      logger.warn({ signal }, 'Shutdown already requested, forcing exit');
      process.exit(1);
    }
    shutdownRequested = true;
    logger.info({ signal }, 'Shutting down gracefully');

    const timeout = setTimeout(() => {
      logger.error('Shutdown timeout exceeded, forcing exit');
      process.exit(1);
    }, config.shutdownTimeout);

    try {
      await fastify.close();
      clearTimeout(timeout);
      logger.info('Server closed successfully');
      process.exit(0);
    } catch (error) {
      clearTimeout(timeout);
      logger.error({ err: error }, 'Error during shutdown');
      process.exit(1);
    }
  };

  process.on('SIGINT', () => shutdown('SIGINT'));
  process.on('SIGTERM', () => shutdown('SIGTERM'));

  try {
    await fastify.listen({ host: config.host, port: config.port });
    logger.info({ host: config.host, port: config.port }, 'Server started');
  } catch (error) {
    logger.error({ err: error }, 'Failed to start server');
    process.exit(1);
  }
}
