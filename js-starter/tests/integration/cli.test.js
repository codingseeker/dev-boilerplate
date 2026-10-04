import { test, describe } from 'node:test';
import assert from 'node:assert/strict';
import { spawn } from 'child_process';
import { fileURLToPath } from 'url';
import { dirname, join } from 'path';

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);
const cliPath = join(__dirname, '../../src/cli/index.js');

describe('CLI Integration', () => {
  test('should show help', async () => {
    const child = spawn('node', [cliPath, '--help']);
    let output = '';
    child.stdout.on('data', (data) => {
      output += data.toString();
    });
    await new Promise((resolve) => {
      child.on('close', (code) => {
        assert.equal(code, 0);
        assert.ok(output.includes('help'));
        resolve();
      });
    });
  });

  test('should show version', async () => {
    const child = spawn('node', [cliPath, '--version']);
    let output = '';
    child.stdout.on('data', (data) => {
      output += data.toString();
    });
    await new Promise((resolve) => {
      child.on('close', (code) => {
        assert.equal(code, 0);
        assert.ok(output.length > 0);
        resolve();
      });
    });
  });

  test('should run core application', async () => {
    const child = spawn('node', [cliPath, 'run']);
    child.stdout.on('data', () => {});
    await new Promise((resolve) => {
      child.on('close', (code) => {
        assert.equal(code, 0);
        resolve();
      });
    });
  });
});
