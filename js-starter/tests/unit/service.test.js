import { test, describe } from 'node:test';
import assert from 'node:assert/strict';
import { createDemoService } from '../../src/services/demoService.js';

describe('DemoService', () => {
  test('should return demo message', async () => {
    const logger = { debug: () => {} };
    const service = createDemoService(logger);
    const result = await service.getMessage();
    assert.ok(result.message);
    assert.ok(result.timestamp);
  });
});
