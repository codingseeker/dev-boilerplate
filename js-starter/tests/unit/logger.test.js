import { test, describe } from 'node:test';
import assert from 'node:assert/strict';
import { createLogger } from '../../src/core/logger.js';

describe('Logger', () => {
  test('should create logger with correct level', () => {
    const config = { nodeEnv: 'development', logLevel: 'debug' };
    const logger = createLogger(config);
    assert.ok(logger);
    assert.equal(logger.level, 'debug');
  });

  test('should create production logger', () => {
    const config = { nodeEnv: 'production', logLevel: 'info' };
    const logger = createLogger(config);
    assert.ok(logger);
    assert.equal(logger.level, 'info');
  });
});
