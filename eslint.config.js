import js from "@eslint/js";
import reactHooks from "eslint-plugin-react-hooks";
import reactRefresh from "eslint-plugin-react-refresh";
import globals from "globals";
import tseslint from "typescript-eslint";

export default tseslint.config(
  {
    ignores: ["**/dist/**", "**/coverage/**", "**/node_modules/**", "**/wailsjs/**"],
  },
  js.configs.recommended,
  ...tseslint.configs.recommended,
  {
    files: ["eslint.config.js", "scripts/**/*.mjs"],
    languageOptions: {
      globals: {
        ...globals.node,
        ...globals.es2022,
      },
    },
  },
  {
    files: ["**/*.{ts,tsx}"],
    languageOptions: {
      globals: {
        ...globals.browser,
        ...globals.es2022,
      },
    },
    plugins: {
      "react-hooks": reactHooks,
      "react-refresh": reactRefresh,
    },
    rules: {
      ...reactHooks.configs.flat.recommended.rules,
      "react-refresh/only-export-components": ["warn", { allowConstantExport: true }],
    },
  },
  {
    files: [
      "apps/desktop/frontend/src/screens/**/*.tsx",
      "apps/desktop/frontend/src/components/**/*.tsx",
    ],
    rules: {
      complexity: ["error", 24],
      "max-depth": ["error", 4],
      "max-lines": ["error", { max: 600, skipBlankLines: true, skipComments: true }],
      "max-lines-per-function": [
        "error",
        { max: 300, skipBlankLines: true, skipComments: true, IIFEs: true },
      ],
      "no-restricted-imports": [
        "error",
        {
          patterns: [
            {
              group: ["**/wailsjs/**"],
              message:
                "Use a typed data adapter; screens and components do not own the Wails bridge.",
            },
          ],
        },
      ],
    },
  },
);
