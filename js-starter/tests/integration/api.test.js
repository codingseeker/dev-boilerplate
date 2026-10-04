import { test, describe, after } from 'node:test';
import assert from 'node:assert/strict';
import { loadConfig } from '../../src/core/config.js';
import { createLogger } from '../../src/core/logger.js';
import { createDemoService } from '../../src/services/demoService.js';
import { createApp } from '../../src/api/app.js';

describe('API Integration', () => {
  const config = loadConfig({ nodeEnv: 'testing', port: 0 });
  const logger = createLogger({ ...config, logLevel: 'error' });
  const demoService = createDemoService(logger);
  const services = { demoService };
  let app;

  test('should start app and respond to health', async () => {
    app = await createApp(config, logger, services);
    const response = await app.inject({
      method: 'GET',
      url: '/health',
    });
    assert.equal(response.statusCode, 200);
    const body = JSON.parse(response.body);
    assert.equal(body.status, 'ok');
  });

  test('should respond to ready', async () => {
    const response = await app.inject({
      method: 'GET',
      url: '/ready',
    });
    assert.equal(response.statusCode, 200);
    const body = JSON.parse(response.body);
    assert.equal(body.status, 'ready');
  });

  test('should respond to demo endpoint', async () => {
    const response = await app.inject({
      method: 'GET',
      url: '/demo',
    });
    assert.equal(response.statusCode, 200);
    const body = JSON.parse(response.body);
    assert.ok(body.message);
  });

  test('should return 404 for unknown route', async () => {
    const response = await app.inject({
      method: 'GET',
      url: '/unknown',
    });
    assert.equal(response.statusCode, 404);
  });

  after(async () => {
    if (app) {
      await app.close();
    }
  });
});
