#!/usr/bin/env node
'use strict';

const childProcess = require('node:child_process');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');

const rootDir = path.resolve(__dirname, '..');

const repository = {
  type: 'git',
  url: 'git+https://github.com/openminutes/openminutes.git',
};

const bugs = {
  url: 'https://github.com/openminutes/openminutes/issues',
};

const homepage = 'https://github.com/openminutes/openminutes#readme';
const license = 'GPL-3.0-only';
const mainPackageName = '@openminutes/cli';

const targets = [
  {
    id: 'darwin_amd64',
    packageName: '@openminutes/cli-darwin-amd64',
    packageDirName: 'cli-darwin-amd64',
    npmOS: 'darwin',
    npmCPU: 'x64',
    binaryName: 'openminutes',
    label: 'macOS x64',
  },
  {
    id: 'darwin_arm64',
    packageName: '@openminutes/cli-darwin-arm64',
    packageDirName: 'cli-darwin-arm64',
    npmOS: 'darwin',
    npmCPU: 'arm64',
    binaryName: 'openminutes',
    label: 'macOS arm64',
  },
  {
    id: 'linux_386',
    packageName: '@openminutes/cli-linux-386',
    packageDirName: 'cli-linux-386',
    npmOS: 'linux',
    npmCPU: 'ia32',
    binaryName: 'openminutes',
    label: 'Linux ia32',
  },
  {
    id: 'linux_amd64',
    packageName: '@openminutes/cli-linux-amd64',
    packageDirName: 'cli-linux-amd64',
    npmOS: 'linux',
    npmCPU: 'x64',
    binaryName: 'openminutes',
    label: 'Linux x64',
  },
  {
    id: 'linux_arm64',
    packageName: '@openminutes/cli-linux-arm64',
    packageDirName: 'cli-linux-arm64',
    npmOS: 'linux',
    npmCPU: 'arm64',
    binaryName: 'openminutes',
    label: 'Linux arm64',
  },
  {
    id: 'windows_386',
    packageName: '@openminutes/cli-windows-386',
    packageDirName: 'cli-windows-386',
    npmOS: 'win32',
    npmCPU: 'ia32',
    binaryName: 'openminutes.exe',
    label: 'Windows ia32',
  },
  {
    id: 'windows_amd64',
    packageName: '@openminutes/cli-windows-amd64',
    packageDirName: 'cli-windows-amd64',
    npmOS: 'win32',
    npmCPU: 'x64',
    binaryName: 'openminutes.exe',
    label: 'Windows x64',
  },
  {
    id: 'windows_arm64',
    packageName: '@openminutes/cli-windows-arm64',
    packageDirName: 'cli-windows-arm64',
    npmOS: 'win32',
    npmCPU: 'arm64',
    binaryName: 'openminutes.exe',
    label: 'Windows arm64',
  },
];

const semverPattern =
  /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9A-Za-z-][0-9A-Za-z-]*))*))?(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$/;

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
    'Usage: node scripts/build-npm-packages.js [--version v0.1.2] [--artifacts dist] [--out dist/npm]',
    '',
    'Builds npm package staging directories from GoReleaser artifacts.',
    'The version defaults to GITHUB_REF_NAME and may include a leading v.',
  ].join('\n');
}

function normalizeVersion(rawVersion) {
  const versionSource = rawVersion || process.env.GITHUB_REF_NAME;
  if (!versionSource) {
    throw new Error('Missing version. Pass --version or set GITHUB_REF_NAME.');
  }

  const version = versionSource.startsWith('v') ? versionSource.slice(1) : versionSource;
  if (!semverPattern.test(version)) {
    throw new Error(`Invalid npm semver version after stripping leading v: ${version}`);
  }

  return version;
}

function writeJSON(filePath, data) {
  fs.writeFileSync(filePath, `${JSON.stringify(data, null, 2)}\n`);
}

function ensureDirectory(dir) {
  fs.mkdirSync(dir, { recursive: true });
}

function removeDirectory(dir) {
  fs.rmSync(dir, { recursive: true, force: true });
}

function copyLicense(packageDir) {
  fs.copyFileSync(path.join(rootDir, 'LICENSE'), path.join(packageDir, 'LICENSE'));
}

function walkFiles(dir) {
  const entries = fs.readdirSync(dir, { withFileTypes: true });
  const files = [];

  for (const entry of entries) {
    const fullPath = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      files.push(...walkFiles(fullPath));
    } else if (entry.isFile()) {
      files.push(fullPath);
    }
  }

  return files;
}

function stripArchiveExtension(filePath) {
  const basename = path.basename(filePath);
  if (basename.endsWith('.tar.gz')) {
    return basename.slice(0, -'.tar.gz'.length);
  }
  if (basename.endsWith('.tgz')) {
    return basename.slice(0, -'.tgz'.length);
  }
  if (basename.endsWith('.zip')) {
    return basename.slice(0, -'.zip'.length);
  }

  return basename;
}

function extractArchive(archivePath, destination) {
  ensureDirectory(destination);

  if (archivePath.endsWith('.tar.gz') || archivePath.endsWith('.tgz')) {
    childProcess.execFileSync('tar', ['-xzf', archivePath, '-C', destination], {
      stdio: ['ignore', 'pipe', 'pipe'],
    });
    return;
  }

  if (archivePath.endsWith('.zip')) {
    childProcess.execFileSync('unzip', ['-q', archivePath, '-d', destination], {
      stdio: ['ignore', 'pipe', 'pipe'],
    });
    return;
  }

  throw new Error(`Unsupported archive: ${archivePath}`);
}

function prepareSearchRoots(artifactsDir) {
  const searchRoots = [artifactsDir];
  const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), 'openminutes-npm-artifacts-'));
  let extractedCount = 0;

  for (const filePath of walkFiles(artifactsDir)) {
    if (
      !filePath.endsWith('.tar.gz') &&
      !filePath.endsWith('.tgz') &&
      !filePath.endsWith('.zip')
    ) {
      continue;
    }

    const destination = path.join(tempDir, stripArchiveExtension(filePath));
    try {
      extractArchive(filePath, destination);
      extractedCount += 1;
    } catch (error) {
      throw new Error(`Failed to extract ${filePath}: ${error.message}`);
    }
  }

  if (extractedCount > 0) {
    searchRoots.push(tempDir);
  }

  return {
    roots: searchRoots,
    cleanup() {
      removeDirectory(tempDir);
    },
  };
}

function findBinary(searchRoots, target) {
  const matches = [];

  for (const searchRoot of searchRoots) {
    for (const filePath of walkFiles(searchRoot)) {
      const normalizedPath = filePath.split(path.sep).join('/');
      if (path.basename(filePath) === target.binaryName && normalizedPath.includes(target.id)) {
        matches.push(filePath);
      }
    }
  }

  if (matches.length === 0) {
    throw new Error(
      `Could not find ${target.binaryName} for ${target.id}. ` +
        'Run goreleaser release --snapshot --clean before building npm packages.',
    );
  }

  matches.sort((left, right) => left.length - right.length || left.localeCompare(right));
  return matches[0];
}

function platformPackageDir(outDir, target) {
  return path.join(outDir, 'platforms', target.packageDirName);
}

function mainPackageDir(outDir) {
  return path.join(outDir, 'main');
}

function commonManifest(name, version, description) {
  return {
    name,
    version,
    description,
    license,
    private: false,
    repository,
    bugs,
    homepage,
  };
}

function writePlatformPackage(outDir, target, version, binarySourcePath) {
  const packageDir = platformPackageDir(outDir, target);
  const binDir = path.join(packageDir, 'bin');
  const binaryDestinationPath = path.join(binDir, target.binaryName);

  ensureDirectory(binDir);
  fs.copyFileSync(binarySourcePath, binaryDestinationPath);
  fs.chmodSync(binaryDestinationPath, 0o755);
  copyLicense(packageDir);

  writeJSON(path.join(packageDir, 'package.json'), {
    ...commonManifest(
      target.packageName,
      version,
      `OpenMinutes CLI binary for ${target.label}.`,
    ),
    os: [target.npmOS],
    cpu: [target.npmCPU],
    files: ['bin/', 'README.md', 'LICENSE'],
  });

  fs.writeFileSync(
    path.join(packageDir, 'README.md'),
    [
      `# ${target.packageName}`,
      '',
      `This package contains the OpenMinutes CLI binary for ${target.label}.`,
      'It is installed automatically by `@openminutes/cli` as an optional dependency.',
      '',
    ].join('\n'),
  );

  return packageDir;
}

function writeMainPackage(outDir, version) {
  const packageDir = mainPackageDir(outDir);
  const binDir = path.join(packageDir, 'bin');
  const optionalDependencies = {};

  for (const target of targets) {
    optionalDependencies[target.packageName] = version;
  }

  ensureDirectory(binDir);
  fs.copyFileSync(
    path.join(rootDir, 'npm', 'bin', 'openminutes.js'),
    path.join(binDir, 'openminutes.js'),
  );
  fs.chmodSync(path.join(binDir, 'openminutes.js'), 0o755);
  copyLicense(packageDir);

  writeJSON(path.join(packageDir, 'package.json'), {
    ...commonManifest(mainPackageName, version, 'OpenMinutes CLI for Feishu/Lark Minutes.'),
    type: 'commonjs',
    bin: {
      openminutes: 'bin/openminutes.js',
    },
    files: ['bin/', 'README.md', 'LICENSE'],
    optionalDependencies,
  });

  fs.writeFileSync(
    path.join(packageDir, 'README.md'),
    [
      '# @openminutes/cli',
      '',
      'OpenMinutes is a CLI for managing Feishu/Lark Minutes.',
      '',
      'Install globally with npm:',
      '',
      '```sh',
      'npm install -g @openminutes/cli',
      '```',
      '',
      'The package installs a small Node.js launcher plus a platform-specific Go binary package.',
      '',
    ].join('\n'),
  );

  return packageDir;
}

function buildPackages(options = {}) {
  const version = normalizeVersion(options.version);
  const artifactsDir = path.resolve(options.artifactsDir || path.join(rootDir, 'dist'));
  const outDir = path.resolve(options.outDir || path.join(rootDir, 'dist', 'npm'));

  if (!fs.existsSync(artifactsDir)) {
    throw new Error(`Artifacts directory does not exist: ${artifactsDir}`);
  }

  removeDirectory(outDir);
  ensureDirectory(outDir);

  const searchRoots = prepareSearchRoots(artifactsDir);
  const platformDirs = [];

  try {
    for (const target of targets) {
      const binarySourcePath = findBinary(searchRoots.roots, target);
      platformDirs.push({
        target,
        packageDir: writePlatformPackage(outDir, target, version, binarySourcePath),
        binarySourcePath,
      });
    }

    const mainDir = writeMainPackage(outDir, version);

    return {
      version,
      artifactsDir,
      outDir,
      mainDir,
      platformDirs,
    };
  } finally {
    searchRoots.cleanup();
  }
}

function main() {
  const options = parseArgs(process.argv.slice(2));
  if (options.help) {
    console.log(usage());
    return;
  }

  const result = buildPackages(options);
  console.log(`Built npm packages for ${result.version} in ${result.outDir}`);
  console.log(`- ${mainPackageName}: ${result.mainDir}`);
  for (const platformDir of result.platformDirs) {
    console.log(`- ${platformDir.target.packageName}: ${platformDir.packageDir}`);
  }
}

if (require.main === module) {
  try {
    main();
  } catch (error) {
    console.error(error.message);
    process.exit(1);
  }
}

module.exports = {
  buildPackages,
  mainPackageDir,
  mainPackageName,
  normalizeVersion,
  platformPackageDir,
  targets,
};
