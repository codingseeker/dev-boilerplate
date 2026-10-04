import { test, describe, afterEach } from 'node:test';
import assert from 'node:assert/strict';
import { loadConfig } from '../../src/core/config.js';
import { ConfigurationError } from '../../src/core/errors.js';

describe('Configuration', () => {
  const originalEnv = process.env;

  afterEach(() => {
    process.env = originalEnv;
  });

  test('should load default configuration', () => {
    process.env = {};
    const config = loadConfig();
    assert.equal(config.appName, 'js-starter');
    assert.equal(config.nodeEnv, 'development');
    assert.equal(config.logLevel, 'info');
    assert.equal(config.port, 3000);
  });

  test('should override with environment variables', () => {
    process.env = {
      APP_NAME: 'test-app',
      NODE_ENV: 'production',
      LOG_LEVEL: 'debug',
      PORT: '8080',
    };
    const config = loadConfig();
    assert.equal(config.appName, 'test-app');
    assert.equal(config.nodeEnv, 'production');
    assert.equal(config.logLevel, 'debug');
    assert.equal(config.port, 8080);
  });

  test('should override with explicit overrides', () => {
    process.env = {};
    const config = loadConfig({ appName: 'override-app', port: 4000 });
    assert.equal(config.appName, 'override-app');
    assert.equal(config.port, 4000);
  });

  test('should throw on invalid configuration', () => {
    process.env = { PORT: 'invalid' };
    assert.throws(() => loadConfig(), ConfigurationError);
  });
});
