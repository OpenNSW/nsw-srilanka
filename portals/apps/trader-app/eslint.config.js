import js from '@eslint/js'
import globals from 'globals'
import reactHooks from 'eslint-plugin-react-hooks'
import reactRefresh from 'eslint-plugin-react-refresh'
import tseslint from 'typescript-eslint'
import { defineConfig, globalIgnores } from 'eslint/config'
import eslintPluginPath from 'eslint-plugin-path'

export default defineConfig([
  globalIgnores(['dist']),
  {
    files: ['**/*.{ts,tsx}'],
    extends: [
      js.configs.recommended,
      tseslint.configs.recommendedTypeChecked, // Use type-checked rules
      reactHooks.configs.flat.recommended,
      reactRefresh.configs.vite,
    ],
    plugins: { path: eslintPluginPath },
    rules: {
      'path/no-relative-imports': ['error', { maxDepth: 0 }],
    },
    settings: {
      path: { config: 'tsconfig.app.json' },
    },
    languageOptions: {
      ecmaVersion: 2020,
      globals: globals.browser,
      parserOptions: {
        project: ['./tsconfig.app.json', './tsconfig.node.json'],
        tsconfigRootDir: import.meta.dirname,
      },
    },
  },
])
