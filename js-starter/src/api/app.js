import Fastify from 'fastify';
import { errorHandler } from './errors.js';
import { healthRoutes } from './routes/health.js';
import { demoRoutes } from './routes/demo.js';

export async function createApp(config, logger, services) {
  const fastify = Fastify({
    logger: false,
    logLevel: 'silent',
    bodyLimit: 1048576,
    requestTimeout: config.httpRequestTimeout,
    keepAliveTimeout: config.httpKeepAliveTimeout,
    connectionTimeout: config.httpRequestTimeout,
  });

  fastify.setErrorHandler(errorHandler);

  fastify.register(healthRoutes);
  fastify.register(demoRoutes, { demoService: services.demoService });

  return fastify;
}
