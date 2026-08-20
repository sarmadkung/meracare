// Metro configuration for the pnpm workspace.
// https://docs.expo.dev/guides/monorepos/
const { getDefaultConfig } = require('expo/metro-config');
const path = require('node:path');

const projectRoot = __dirname;
const workspaceRoot = path.resolve(projectRoot, '../..');

const config = getDefaultConfig(projectRoot);

// Watch the workspace so `@meracare/contracts` source is transformed by Metro.
config.watchFolders = [workspaceRoot];

config.resolver.nodeModulesPaths = [
  path.resolve(projectRoot, 'node_modules'),
  path.resolve(workspaceRoot, 'node_modules'),
];
// Resolve every package from the app's own tree first, so a single React copy
// is used across the workspace.
config.resolver.disableHierarchicalLookup = true;

// expo-sqlite ships its web implementation as a WebAssembly build of SQLite,
// which Metro will not resolve unless `wasm` is a known asset extension.
// Without this, any web bundle that reaches src/lib/offline/database.ts fails
// on `./wa-sqlite/wa-sqlite.wasm` even though the file is present.
config.resolver.assetExts.push('wasm');

// That same web build stores the database in a SharedArrayBuffer, which
// browsers only expose to cross-origin isolated pages. The dev server has to
// send these headers or the worker cannot start.
config.server.enhanceMiddleware = (middleware) => {
  return (req, res, next) => {
    res.setHeader('Cross-Origin-Embedder-Policy', 'credentialless');
    res.setHeader('Cross-Origin-Opener-Policy', 'same-origin');
    middleware(req, res, next);
  };
};

module.exports = config;
