import { z } from 'zod';
import dotenv from 'dotenv';
import { ConfigurationError } from './errors.js';

const configSchema = z.object({
  appName: z.string().default('js-starter'),
  nodeEnv: z
    .enum(['development', 'testing', 'production'])
    .default('development'),
  logLevel: z.enum(['debug', 'info', 'warn', 'error']).default('info'),
  host: z.string().default('0.0.0.0'),
  port: z.number().int().min(0).max(65535).default(3000),
  httpRequestTimeout: z.number().int().min(1000).max(300000).default(5000),
  httpHeadersTimeout: z.number().int().min(1000).max(300000).default(6000),
  httpKeepAliveTimeout: z.number().int().min(1000).max(300000).default(5000),
  shutdownTimeout: z.number().int().min(1000).max(60000).default(10000),
});

export function loadConfig(overrides = {}) {
  const env = process.env.NODE_ENV;
  if (env !== 'production') {
    dotenv.config();
  }

  const config = {
    appName: process.env.APP_NAME,
    nodeEnv: process.env.NODE_ENV,
    logLevel: process.env.LOG_LEVEL,
    host: process.env.HOST,
    port: process.env.PORT ? parseInt(process.env.PORT, 10) : undefined,
    httpRequestTimeout: process.env.HTTP_REQUEST_TIMEOUT
      ? parseInt(process.env.HTTP_REQUEST_TIMEOUT, 10)
      : undefined,
    httpHeadersTimeout: process.env.HTTP_HEADERS_TIMEOUT
      ? parseInt(process.env.HTTP_HEADERS_TIMEOUT, 10)
      : undefined,
    httpKeepAliveTimeout: process.env.HTTP_KEEP_ALIVE_TIMEOUT
      ? parseInt(process.env.HTTP_KEEP_ALIVE_TIMEOUT, 10)
      : undefined,
    shutdownTimeout: process.env.SHUTDOWN_TIMEOUT
      ? parseInt(process.env.SHUTDOWN_TIMEOUT, 10)
      : undefined,
    ...overrides,
  };

  try {
    return configSchema.parse(config);
  } catch (error) {
    if (error instanceof z.ZodError) {
      const message = error.errors
        .map((e) => `${e.path.join('.')}: ${e.message}`)
        .join('; ');
      throw new ConfigurationError(`Invalid configuration: ${message}`, {
        cause: error,
      });
    }
    throw error;
  }
}
