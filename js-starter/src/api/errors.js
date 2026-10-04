export function errorHandler(request, reply) {
  const error = request.error || new Error('Unknown error');

  request.log.error({ err: error }, 'Request error');

  const isValidationError =
    error.validation ||
    error.statusCode === 400 ||
    error.code === 'VALIDATION_ERROR';
  const statusCode = error.statusCode || (isValidationError ? 400 : 500);

  const response = {
    error: {
      message: statusCode < 500 ? error.message : 'Internal Server Error',
      code: error.code || 'INTERNAL_ERROR',
    },
  };

  reply.code(statusCode).send(response);
}
