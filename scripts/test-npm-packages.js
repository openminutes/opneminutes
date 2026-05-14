#!/usr/bin/env node
'use strict';

const childProcess = require('node:child_process');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');

const {
  buildPackages,
  mainPackageDir,
  mainPackageName,
  platformPackageDir,
  targets,
} = require('./build-npm-packages.js');

function parseArgs(argv) {
  const options = {};

  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];

    if (arg === '--version') {
      options.version = argv[++index];
    } else if (arg.startsWith('--version=')) {
      options.version = arg.slice('--version='.length);
    } else if (arg === '--artifacts') {
      options.artifactsDir = argv[++index];
    } else if (arg.startsWith('--artifacts=')) {
      options.artifactsDir = arg.slice('--artifacts='.length);
    } else if (arg === '--out') {
      options.outDir = argv[++index];
    } else if (arg.startsWith('--out=')) {
      options.outDir = arg.slice('--out='.length);
    } else if (arg === '--skip-pack') {
      options.skipPack = true;
    } else if (arg === '--skip-install') {
      options.skipInstall = true;
    } else if (arg === '--help' || arg === '-h') {
      options.help = true;
    } else {
      throw new Error(`Unknown argument: ${arg}`);
    }
  }

  return options;
}

function usage() {
  return [
    'Usage: node scripts/test-npm-packages.js [--version 0.0.0-test.0] [--artifacts dist] [--out dist/npm]',
    '',
    'Validates generated npm packages from GoReleaser artifacts.',
    'Run goreleaser release --snapshot --clean first unless dist already contains artifacts.',
  ].join('\n');
}

function readJSON(filePath) {
  return JSON.parse(fs.readFileSync(filePath, 'utf8'));
}

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

function assertFile(filePath) {
  assert(fs.existsSync(filePath), `Expected file to exist: ${filePath}`);
  assert(fs.statSync(filePath).isFile(), `Expected regular file: ${filePath}`);
}

function verifyPackageFiles(result) {
  const mainDir = mainPackageDir(result.outDir);
  const mainManifest = readJSON(path.join(mainDir, 'package.json'));

  assert(mainManifest.name === mainPackageName, 'Main package has the wrong name.');
  assert(mainManifest.version === result.version, 'Main package has the wrong version.');
  assert(mainManifest.private === false, 'Main package must be publishable.');
  assert(mainManifest.license === 'GPL-3.0-only', 'Main package has the wrong license.');
  assert(mainManifest.bin.openminutes === 'bin/openminutes.js', 'Main package bin is wrong.');
  assertFile(path.join(mainDir, 'bin', 'openminutes.js'));
  assertFile(path.join(mainDir, 'README.md'));
  assertFile(path.join(mainDir, 'LICENSE'));

  for (const target of targets) {
    assert(
      mainManifest.optionalDependencies[target.packageName] === result.version,
      `Main package optional dependency is missing or wrong: ${target.packageName}`,
    );
  }

  for (const target of targets) {
    const packageDir = platformPackageDir(result.outDir, target);
    const manifest = readJSON(path.join(packageDir, 'package.json'));
    const binaryPath = path.join(packageDir, 'bin', target.binaryName);

    assert(manifest.name === target.packageName, `${target.packageName} has the wrong name.`);
    assert(manifest.version === result.version, `${target.packageName} has the wrong version.`);
    assert(manifest.private === false, `${target.packageName} must be publishable.`);
    assert(manifest.license === 'GPL-3.0-only', `${target.packageName} has the wrong license.`);
    assert(manifest.os.length === 1 && manifest.os[0] === target.npmOS, `${target.packageName} os is wrong.`);
    assert(
      manifest.cpu.length === 1 && manifest.cpu[0] === target.npmCPU,
      `${target.packageName} cpu is wrong.`,
    );
    assertFile(binaryPath);
    assert(fs.statSync(binaryPath).size > 0, `Binary is empty: ${binaryPath}`);
    assertFile(path.join(packageDir, 'README.md'));
    assertFile(path.join(packageDir, 'LICENSE'));
  }
}

function npmPackDryRun(packageDir) {
  const output = childProcess.execFileSync('npm', ['pack', '--dry-run', '--json'], {
    cwd: packageDir,
    encoding: 'utf8',
    stdio: ['ignore', 'pipe', 'pipe'],
  });
  const packs = JSON.parse(output);
  assert(Array.isArray(packs) && packs.length === 1, `Unexpected npm pack output for ${packageDir}`);
  return new Set(packs[0].files.map((file) => file.path));
}

function npmPack(packageDir, destination) {
  const output = childProcess.execFileSync(
    'npm',
    ['pack', '--json', '--pack-destination', destination],
    {
      cwd: packageDir,
      encoding: 'utf8',
      stdio: ['ignore', 'pipe', 'pipe'],
    },
  );
  const packs = JSON.parse(output);
  assert(Array.isArray(packs) && packs.length === 1, `Unexpected npm pack output for ${packageDir}`);
  return path.join(destination, packs[0].filename);
}

function verifyDryRunPacks(result) {
  const mainFiles = npmPackDryRun(result.mainDir);
  assert(mainFiles.has('bin/openminutes.js'), 'Main npm pack is missing bin/openminutes.js.');
  assert(mainFiles.has('package.json'), 'Main npm pack is missing package.json.');
  assert(mainFiles.has('README.md'), 'Main npm pack is missing README.md.');
  assert(mainFiles.has('LICENSE'), 'Main npm pack is missing LICENSE.');

  for (const target of targets) {
    const packageDir = platformPackageDir(result.outDir, target);
    const files = npmPackDryRun(packageDir);
    assert(files.has(`bin/${target.binaryName}`), `${target.packageName} pack is missing its binary.`);
    assert(files.has('package.json'), `${target.packageName} pack is missing package.json.`);
    assert(files.has('README.md'), `${target.packageName} pack is missing README.md.`);
    assert(files.has('LICENSE'), `${target.packageName} pack is missing LICENSE.`);
  }
}

function currentTarget() {
  return targets.find((target) => target.npmOS === process.platform && target.npmCPU === process.arch);
}

function installAndRunCurrentPackage(result) {
  const target = currentTarget();
  if (!target) {
    console.warn(`Skipping install test for unsupported platform ${process.platform}/${process.arch}.`);
    return;
  }

  const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), 'openminutes-npm-install-'));

  try {
    const mainTarball = npmPack(result.mainDir, tempDir);
    const platformTarball = npmPack(platformPackageDir(result.outDir, target), tempDir);

    fs.writeFileSync(
      path.join(tempDir, 'package.json'),
      `${JSON.stringify(
        {
          private: true,
          dependencies: {
            [mainPackageName]: `file:${mainTarball}`,
            [target.packageName]: `file:${platformTarball}`,
          },
        },
        null,
        2,
      )}\n`,
    );

    childProcess.execFileSync('npm', ['install', '--ignore-scripts'], {
      cwd: tempDir,
      stdio: ['ignore', 'pipe', 'pipe'],
    });

    const executable = path.join(
      tempDir,
      'node_modules',
      '.bin',
      process.platform === 'win32' ? 'openminutes.cmd' : 'openminutes',
    );
    const run = childProcess.spawnSync(executable, ['--version'], {
      cwd: tempDir,
      encoding: 'utf8',
    });
    const output = `${run.stdout || ''}${run.stderr || ''}`;

    assert(run.status === 0, `openminutes --version failed:\n${output}`);
    assert(/openminutes version /.test(output), `Unexpected openminutes --version output:\n${output}`);
  } finally {
    fs.rmSync(tempDir, { recursive: true, force: true });
  }
}

function defaultVersion() {
  const refName = process.env.GITHUB_REF_NAME || '';
  if (/^v?\d+\.\d+\.\d+/.test(refName)) {
    return refName;
  }

  return '0.0.0-test.0';
}

function main() {
  const options = parseArgs(process.argv.slice(2));
  if (options.help) {
    console.log(usage());
    return;
  }

  const result = buildPackages({
    version: options.version || defaultVersion(),
    artifactsDir: options.artifactsDir,
    outDir: options.outDir,
  });

  verifyPackageFiles(result);

  if (!options.skipPack) {
    verifyDryRunPacks(result);
  }

  if (!options.skipInstall) {
    installAndRunCurrentPackage(result);
  }

  console.log(`Validated npm packages for ${result.version} in ${result.outDir}`);
}

if (require.main === module) {
  try {
    main();
  } catch (error) {
    console.error(error.message);
    process.exit(1);
  }
}
