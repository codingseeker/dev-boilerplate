export class AppError extends Error {
  constructor(message, options = {}) {
    super(message);
    this.name = 'AppError';
    this.code = options.code || 'APP_ERROR';
    this.statusCode = options.statusCode || 500;
    this.cause = options.cause;
  }
}

export class ValidationError extends AppError {
  constructor(message, options = {}) {
    super(message, { code: 'VALIDATION_ERROR', statusCode: 400, ...options });
    this.name = 'ValidationError';
  }
}

export class ConfigurationError extends AppError {
  constructor(message, options = {}) {
    super(message, { code: 'CONFIG_ERROR', statusCode: 500, ...options });
    this.name = 'ConfigurationError';
  }
}
