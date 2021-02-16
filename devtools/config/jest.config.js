let config = {
  preset: 'ts-jest',
  rootDir: '../../',
  globals: {
    'ts-jest': {
      isolatedModules: true,
    },
  },
  moduleFileExtensions: ['ts', 'js'],
};

// eslint-disable-next-line no-undef
const e2e = process.argv.includes('e2e');
if (e2e) {
  config = {
    ...config,
    setupFilesAfterEnv: ['<rootDir>/devtools/config/e2e-test-setup.ts'],
    testEnvironment: 'node',
  };
}

// eslint-disable-next-line no-undef
module.exports = config;
