import globals from 'globals'
import js from '@eslint/js'
import pluginQuery from '@tanstack/eslint-plugin-query'
import reactHooks from 'eslint-plugin-react-hooks'
import reactRefresh from 'eslint-plugin-react-refresh'
import { defineConfig } from 'eslint/config'
import tseslint from 'typescript-eslint'

export default defineConfig(
  { ignores: ['dist', 'src/components/ui'] },
  {
    extends: [
      js.configs.recommended,
      ...tseslint.configs.recommended,
      ...pluginQuery.configs['flat/recommended'],
    ],
    files: ['**/*.{ts,tsx}'],
    languageOptions: {
      ecmaVersion: 2020,
      globals: globals.browser,
    },
    plugins: {
      'react-hooks': reactHooks,
      'react-refresh': reactRefresh,
    },
    rules: {
      ...reactHooks.configs.recommended.rules,
      'react-refresh/only-export-components': [
        'warn',
        { allowConstantExport: true },
      ],
      'no-console': 'warn', // 改为警告而不是错误
      'prefer-const': 'warn', // 改为警告
      '@typescript-eslint/no-explicit-any': 'warn', // 允许 any 类型但显示警告
      '@typescript-eslint/ban-ts-comment': 'warn', // 允许 @ts-nocheck 和 @ts-ignore，但显示警告
      'react-hooks/set-state-in-effect': 'warn', // setState 在 effect 中调用改为警告
      'react-compiler/react-compiler': 'off', // 禁用 React 编译器检查
      '@tanstack/query/exhaustive-deps': 'warn', // TanStack Query 依赖检查改为警告
      'no-unused-vars': 'off',
      '@typescript-eslint/no-unused-vars': [
        'warn', // 改为警告
        {
          args: 'all',
          argsIgnorePattern: '^_',
          caughtErrors: 'all',
          caughtErrorsIgnorePattern: '^_',
          destructuredArrayIgnorePattern: '^_',
          varsIgnorePattern: '^_',
          ignoreRestSiblings: true,
        },
      ],
      // Enforce type-only imports for TypeScript types
      '@typescript-eslint/consistent-type-imports': [
        'warn', // 改为警告
        {
          prefer: 'type-imports',
          fixStyle: 'inline-type-imports',
          disallowTypeAnnotations: false,
        },
      ],
      // Prevent duplicate imports from the same module
      'no-duplicate-imports': 'warn', // 改为警告
    },
  }
)
