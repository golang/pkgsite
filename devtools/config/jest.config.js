let config = {
  preset: 'ts-jest',
  rootDir: '../../',
  globals: {
    'ts-jest': {
      isolatedModules: true,
    },
  },
  moduleFileExtensions: ['ts', 'js'],
  testRunner: 'jest-circus/runner',
};

// eslint-disable-next-line no-undef
const e2e = process.argv.includes('e2e');
if (e2e) {
  config = {
    ...config,
    setupFilesAfterEnv: ['<rootDir>/e2e/setup.ts'],
    globalSetup: '<rootDir>/e2e/global-setup.ts',
    globalTeardown: '<rootDir>/e2e/global-teardown.ts',
    testEnvironment: 'node',
  };
}

// eslint-disable-next-line no-undef
module.exports = config;
