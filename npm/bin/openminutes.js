#!/usr/bin/env node
'use strict';

const { spawnSync } = require('node:child_process');
const path = require('node:path');

const targets = {
  'darwin x64': {
    packageName: '@openminutes/cli-darwin-amd64',
    binaryName: 'openminutes',
  },
  'darwin arm64': {
    packageName: '@openminutes/cli-darwin-arm64',
    binaryName: 'openminutes',
  },
  'linux ia32': {
    packageName: '@openminutes/cli-linux-386',
    binaryName: 'openminutes',
  },
  'linux x64': {
    packageName: '@openminutes/cli-linux-amd64',
    binaryName: 'openminutes',
  },
  'linux arm64': {
    packageName: '@openminutes/cli-linux-arm64',
    binaryName: 'openminutes',
  },
  'win32 ia32': {
    packageName: '@openminutes/cli-windows-386',
    binaryName: 'openminutes.exe',
  },
  'win32 x64': {
    packageName: '@openminutes/cli-windows-amd64',
    binaryName: 'openminutes.exe',
  },
  'win32 arm64': {
    packageName: '@openminutes/cli-windows-arm64',
    binaryName: 'openminutes.exe',
  },
};

function supportedPlatforms() {
  return Object.keys(targets)
    .map((key) => key.replace(' ', '/'))
    .join(', ');
}

function fail(message) {
  console.error(message);
  process.exit(1);
}

const target = targets[`${process.platform} ${process.arch}`];
if (!target) {
  fail(
    `openminutes: unsupported platform ${process.platform}/${process.arch}.\n` +
      `Supported platforms: ${supportedPlatforms()}.`,
  );
}

let binaryPath;
try {
  const packageJsonPath = require.resolve(`${target.packageName}/package.json`);
  binaryPath = path.join(path.dirname(packageJsonPath), 'bin', target.binaryName);
} catch (error) {
  if (error && error.code === 'MODULE_NOT_FOUND') {
    fail(
      `openminutes: optional package ${target.packageName} was not installed.\n` +
        'Reinstall @openminutes/cli without --omit=optional or --no-optional.',
    );
  }

  throw error;
}

const result = spawnSync(binaryPath, process.argv.slice(2), {
  stdio: 'inherit',
});

if (result.error) {
  fail(`openminutes: failed to run ${binaryPath}: ${result.error.message}`);
}

if (result.signal) {
  process.kill(process.pid, result.signal);
  process.exit(1);
}

process.exit(result.status === null ? 1 : result.status);
