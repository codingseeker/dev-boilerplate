import pino from 'pino';

const SENSITIVE_FIELDS = [
  'password',
  'token',
  'secret',
  'key',
  'apiKey',
  'authorization',
  'cookie',
];

export function createLogger(config) {
  const isProduction = config.nodeEnv === 'production';

  const redaction = {
    paths: SENSITIVE_FIELDS.map((f) => `*.${f}`).concat(SENSITIVE_FIELDS),
    remove: true,
  };

  const options = {
    level: config.logLevel,
    redact: redaction,
  };

  if (!isProduction) {
    options.transport = {
      target: 'pino-pretty',
      options: {
        colorize: true,
        translateTime: 'HH:MM:ss',
        ignore: 'pid,hostname',
      },
    };
  }

  return pino(options);
}
