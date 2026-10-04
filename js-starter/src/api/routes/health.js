export async function healthRoutes(fastify) {
  fastify.get('/health', async (_request, _reply) => {
    return {
      status: 'ok',
      timestamp: new Date().toISOString(),
      uptime: process.uptime(),
    };
  });

  fastify.get('/ready', async (_request, _reply) => {
    return {
      status: 'ready',
      timestamp: new Date().toISOString(),
    };
  });
}
