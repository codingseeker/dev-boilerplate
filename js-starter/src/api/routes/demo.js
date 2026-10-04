export async function demoRoutes(fastify, options) {
  const { demoService } = options;

  fastify.get('/demo', async (_request, _reply) => {
    const result = await demoService.getMessage();
    return result;
  });
}
